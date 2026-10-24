package iface

import (
	"context"
	"github.com/kamui-project/kamui-cli/internal/storage"
)

type StorageService interface {
	List(context.Context, string) (*storage.Overview, error)
	Create(context.Context, string, string) (*storage.Item, error)
	Delete(context.Context, string, string) error
	ListFiles(context.Context, string, string, string, string, int) (*storage.ObjectPage, error)
	Upload(context.Context, string, string, string, string) (*storage.Upload, error)
	Download(context.Context, string, string, string, string, bool) (int64, error)
	DeleteFile(context.Context, string, string, string) error
}
