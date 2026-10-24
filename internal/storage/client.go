package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const MaxFileSize = 100 * 1024 * 1024

var uuidPattern = regexp.MustCompile("^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$")

func ValidID(id string) bool { return uuidPattern.MatchString(id) }
func ValidateKey(key string) error {
	if key == "" || !utf8.ValidString(key) || len(key) > 1024 || strings.ContainsRune(key, 0) {
		return errors.New("object key must be valid UTF-8, 1-1024 bytes, without NUL")
	}
	return nil
}

type PermitProvider func(context.Context) (*Permit, error)
type Client struct {
	provide PermitProvider
	permit  *Permit
	http    *http.Client
	origin  string
	id      string
	role    string
}

func NewClient(id, role string, provide PermitProvider) *Client {
	return &Client{id: id, role: role, provide: provide, http: &http.Client{Timeout: 2 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (c *Client) authorize(ctx context.Context) (*Permit, error) {
	if c.permit != nil && time.Until(c.permit.ExpiresAt) > 15*time.Second {
		return c.permit, nil
	}
	p, err := c.provide(ctx)
	if err != nil {
		return nil, err
	}
	if p == nil || p.Token == "" || len(p.Token) > 16384 || strings.ContainsAny(p.Token, "\r\n") || p.StorageID != c.id || p.Role != c.role || !ValidID(p.StorageID) || !p.ExpiresAt.After(time.Now()) || time.Until(p.ExpiresAt) > 6*time.Minute {
		return nil, errors.New("invalid storage access permit")
	}
	u, err := url.Parse(p.Endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("storage endpoint must be an HTTPS origin")
	}
	origin := strings.TrimRight(p.Endpoint, "/")
	if c.origin != "" && c.origin != origin {
		return nil, errors.New("storage endpoint changed while transferring; retry the operation")
	}
	c.origin = origin
	c.permit = p
	return p, nil
}
func (c *Client) request(ctx context.Context, method, path string, body io.Reader, size int64, binary bool) (*http.Response, error) {
	p, err := c.authorize(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.origin+"/v1/storages/"+c.id+path, body)
	if err != nil {
		return nil, errors.New("invalid storage request")
	}
	req.ContentLength = size
	req.Header.Set("Authorization", "Bearer "+p.Token)
	req.Header.Set("Content-Type", "application/json")
	if binary {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	key := make([]byte, 16)
	if _, err = rand.Read(key); err != nil {
		return nil, err
	}
	req.Header.Set("Idempotency-Key", hex.EncodeToString(key))
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("storage transfer failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		var detail struct {
			Message string `json:"message"`
			Error   struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(raw, &detail)
		message := detail.Message
		if message == "" {
			message = detail.Error.Message
		}
		if message == "" {
			message = detail.Error.Code
		}
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return nil, fmt.Errorf("storage HTTP %d: %s", resp.StatusCode, message)
	}
	return resp, nil
}
func (c *Client) json(ctx context.Context, method, path string, input, result interface{}) error {
	var reader io.Reader
	var size int64
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
		size = int64(len(raw))
	}
	response, err := c.request(ctx, method, path, reader, size, false)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if result != nil {
		return json.NewDecoder(io.LimitReader(response.Body, 4*1024*1024)).Decode(result)
	}
	_, err = io.Copy(io.Discard, response.Body)
	return err
}
func (c *Client) List(ctx context.Context, prefix, cursor string, limit int) (*ObjectPage, error) {
	query := url.Values{"limit": {fmt.Sprint(limit)}}
	if prefix != "" {
		query.Set("prefix", prefix)
	}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	var result ObjectPage
	err := c.json(ctx, http.MethodGet, "/objects?"+query.Encode(), nil, &result)
	return &result, err
}
func (c *Client) Delete(ctx context.Context, key string) error {
	if err := ValidateKey(key); err != nil {
		return err
	}
	return c.json(ctx, http.MethodDelete, "/objects?"+url.Values{"key": {key}}.Encode(), nil, nil)
}
func (c *Client) Download(ctx context.Context, key string, destination io.Writer) error {
	if err := ValidateKey(key); err != nil {
		return err
	}
	response, err := c.request(ctx, http.MethodGet, "/objects?"+url.Values{"key": {key}}.Encode(), nil, 0, false)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.ContentLength > MaxFileSize {
		return errors.New("download exceeds the 100 MiB object limit")
	}
	copied, err := io.Copy(destination, io.LimitReader(response.Body, MaxFileSize+1))
	if copied > MaxFileSize {
		return errors.New("download exceeds the 100 MiB object limit")
	}
	return err
}
func (c *Client) UploadFile(ctx context.Context, key string, file *os.File) (result *Upload, err error) {
	if err = ValidateKey(key); err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > MaxFileSize {
		return nil, errors.New("upload requires a regular file no larger than 100 MiB")
	}
	var upload Upload
	if err = c.json(ctx, http.MethodPost, "/uploads", map[string]interface{}{"key": key, "size_bytes": info.Size()}, &upload); err != nil {
		return nil, err
	}
	if !ValidID(upload.ID) {
		return nil, errors.New("invalid upload identifier")
	}
	path := "/uploads/" + upload.ID
	// Best-effort abort releases reservations after failures or cancellation.
	defer func() {
		if err != nil {
			cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = c.json(cleanup, http.MethodDelete, path, nil, nil)
		}
	}()
	var partSize int64
	switch upload.Mode {
	case "single":
		if upload.PartCount != 1 {
			return nil, errors.New("invalid single upload response")
		}
		partSize = info.Size()
	case "multipart":
		partSize = upload.PartSizeBytes
		if partSize < 1 || partSize > MaxFileSize || upload.PartCount != int((info.Size()+partSize-1)/partSize) {
			return nil, errors.New("invalid multipart upload response")
		}
	default:
		return nil, errors.New("unsupported upload mode")
	}
	for part, offset := 1, int64(0); part <= upload.PartCount; part++ {
		size := partSize
		if remaining := info.Size() - offset; size > remaining {
			size = remaining
		}
		target := path + "/data"
		if upload.Mode == "multipart" {
			target = fmt.Sprintf("%s/parts/%d", path, part)
		}
		response, e := c.request(ctx, http.MethodPut, target, io.NewSectionReader(file, offset, size), size, true)
		if e != nil {
			return nil, e
		}
		_, e = io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if e != nil {
			return nil, e
		}
		offset += size
	}
	if err = c.json(ctx, http.MethodPost, path+"/complete", struct{}{}, &upload); err != nil {
		return nil, err
	}
	if upload.Status != "completed" {
		return nil, errors.New("storage did not confirm upload completion")
	}
	return &upload, nil
}
