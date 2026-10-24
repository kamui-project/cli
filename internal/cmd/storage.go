package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/AlecAivazis/survey/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

type StorageCommand struct {
	root    *RootCommand
	cmd     *cobra.Command
	project string
}

func NewStorageCommand(root *RootCommand) *StorageCommand {
	s := &StorageCommand{root: root}
	s.cmd = &cobra.Command{Use: "storage", Aliases: []string{"storages"}, Short: "Manage project storage and files",
		Long:    "Manage storage using your Kamui CLI login. File operations use short-lived access permits. No storage token configuration is required.",
		Example: "  kamui storage list -p myproject\n  kamui storage create -p myproject --name assets\n  kamui storage files upload assets ./photo.jpg -p myproject --key images/photo.jpg"}
	s.cmd.PersistentFlags().StringVarP(&s.project, "project", "p", "", "Project name or ID (required)")
	check := func(cmd *cobra.Command, _ []string) error {
		if s.project == "" {
			return fmt.Errorf("--project is required (use kamui projects list)")
		}
		format := resolveOutputFormat(cmd)
		if format != "" && format != "text" && format != "json" {
			return fmt.Errorf("--output must be text or json")
		}
		return nil
	}
	list := &cobra.Command{Use: "list", Short: "List storage, capacity and object counts", Args: cobra.NoArgs, PreRunE: check, RunE: func(cmd *cobra.Command, _ []string) error {
		result, err := s.root.Container().StorageService().List(cmd.Context(), s.project)
		if err != nil {
			return err
		}
		if resolveOutputFormat(cmd) == "json" {
			return storageJSON(cmd, result)
		}
		rows := make([][]string, 0, len(result.Items))
		for _, item := range result.Items {
			rows = append(rows, []string{item.ID, storageText(item.Name), storageText(item.Status), fmt.Sprint(item.CommittedBytes), fmt.Sprint(item.ObjectCount)})
		}
		printTable(cmd.OutOrStdout(), "", []string{"ID", "NAME", "STATUS", "BYTES", "OBJECTS"}, rows)
		if len(rows) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No storage found. Create one with: kamui storage create -p <project> --name <name>")
		}
		return nil
	}}
	var name string
	create := &cobra.Command{Use: "create", Short: "Create storage in a project", Args: cobra.NoArgs, PreRunE: check, RunE: func(cmd *cobra.Command, _ []string) error {
		result, err := s.root.Container().StorageService().Create(cmd.Context(), s.project, name)
		if err != nil {
			return err
		}
		if resolveOutputFormat(cmd) == "json" {
			return storageJSON(cmd, result)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Created storage %s (%s).\n", storageText(result.Name), result.ID)
		return nil
	}}
	create.Flags().StringVar(&name, "name", "", "Storage name (1-80 characters)")
	_ = create.MarkFlagRequired("name")
	var yes bool
	remove := &cobra.Command{Use: "delete <storage>", Short: "Delete an empty storage (name or ID)", Args: cobra.ExactArgs(1), PreRunE: check, RunE: func(cmd *cobra.Command, args []string) error {
		if err := confirmStorageDelete(yes, "Delete storage "+storageText(args[0])+" in project "+storageText(s.project)+"?"); err != nil {
			return err
		}
		if err := s.root.Container().StorageService().Delete(cmd.Context(), s.project, args[0]); err != nil {
			return err
		}
		if resolveOutputFormat(cmd) == "json" {
			return storageJSON(cmd, map[string]interface{}{"status": "deletion_requested", "storage": args[0], "project": s.project})
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Storage deletion requested.")
		return nil
	}}
	remove.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm storage deletion without prompting")
	files := &cobra.Command{Use: "files", Aliases: []string{"objects"}, Short: "List, upload, download and delete files"}
	var prefix, cursor string
	var limit int
	ls := &cobra.Command{Use: "list <storage>", Short: "List one page of files (name or ID)", Args: cobra.ExactArgs(1), PreRunE: check, RunE: func(cmd *cobra.Command, args []string) error {
		result, err := s.root.Container().StorageService().ListFiles(cmd.Context(), s.project, args[0], prefix, cursor, limit)
		if err != nil {
			return err
		}
		if resolveOutputFormat(cmd) == "json" {
			return storageJSON(cmd, result)
		}
		rows := make([][]string, 0, len(result.Items))
		for _, item := range result.Items {
			rows = append(rows, []string{storageText(item.Key), fmt.Sprint(item.SizeBytes), storageText(item.Generation)})
		}
		printTable(cmd.OutOrStdout(), "", []string{"KEY", "BYTES", "GENERATION"}, rows)
		if result.NextCursor != nil && *result.NextCursor != "" {
			fmt.Fprintf(cmd.ErrOrStderr(), "More files available. Next --cursor: %s\n", storageText(*result.NextCursor))
		}
		return nil
	}}
	ls.Flags().StringVar(&prefix, "prefix", "", "Object key prefix")
	ls.Flags().StringVar(&cursor, "cursor", "", "Continuation cursor from the previous page")
	ls.Flags().IntVar(&limit, "limit", 100, "Maximum files in this page (1-1000)")
	var key string
	upload := &cobra.Command{Use: "upload <storage> <local-file>", Short: "Upload a file up to 100 MiB; replaces the same object key", Args: cobra.ExactArgs(2), PreRunE: check, RunE: func(cmd *cobra.Command, args []string) error {
		target := key
		if target == "" {
			target = filepath.Base(args[1])
		}
		result, err := s.root.Container().StorageService().Upload(cmd.Context(), s.project, args[0], args[1], target)
		if err != nil {
			return err
		}
		if resolveOutputFormat(cmd) == "json" {
			return storageJSON(cmd, result)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Uploaded %s (%d bytes).\n", storageText(result.Key), result.SizeBytes)
		return nil
	}}
	upload.Flags().StringVar(&key, "key", "", "Remote object key (defaults to the local file name)")
	var force bool
	download := &cobra.Command{Use: "download <storage> <key> <local-file>", Short: "Download a file with HTTPS verification; preserves existing files by default", Args: cobra.ExactArgs(3), PreRunE: check, RunE: func(cmd *cobra.Command, args []string) error {
		size, err := s.root.Container().StorageService().Download(cmd.Context(), s.project, args[0], args[1], args[2], force)
		if err != nil {
			return err
		}
		if resolveOutputFormat(cmd) == "json" {
			return storageJSON(cmd, map[string]interface{}{"path": args[2], "key": args[1], "size_bytes": size})
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Downloaded %s to %s (%d bytes).\n", storageText(args[1]), storageText(args[2]), size)
		return nil
	}}
	download.Flags().BoolVar(&force, "force", false, "Replace an existing destination file after a successful download")
	var fileYes bool
	rm := &cobra.Command{Use: "delete <storage> <key>", Short: "Permanently delete one file", Args: cobra.ExactArgs(2), PreRunE: check, RunE: func(cmd *cobra.Command, args []string) error {
		if err := confirmStorageDelete(fileYes, "Permanently delete "+storageText(args[1])+" from storage "+storageText(args[0])+"?"); err != nil {
			return err
		}
		if err := s.root.Container().StorageService().DeleteFile(cmd.Context(), s.project, args[0], args[1]); err != nil {
			return err
		}
		if resolveOutputFormat(cmd) == "json" {
			return storageJSON(cmd, map[string]interface{}{"deleted": true, "key": args[1], "storage": args[0]})
		}
		fmt.Fprintln(cmd.OutOrStdout(), "File deleted.")
		return nil
	}}
	rm.Flags().BoolVarP(&fileYes, "yes", "y", false, "Confirm file deletion without prompting")
	files.AddCommand(ls, upload, download, rm)
	s.cmd.AddCommand(list, create, remove, files)
	return s
}
func (s *StorageCommand) Command() *cobra.Command { return s.cmd }
func storageJSON(cmd *cobra.Command, result interface{}) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}
func storageText(value string) string {
	quoted := strconv.QuoteToGraphic(value)
	return quoted[1 : len(quoted)-1]
}
func confirmStorageDelete(yes bool, message string) error {
	if yes {
		return nil
	}
	if !isatty.IsTerminal(os.Stdin.Fd()) && !isatty.IsCygwinTerminal(os.Stdin.Fd()) {
		return fmt.Errorf("deletion requires --yes when stdin is not a terminal")
	}
	confirmed := false
	if err := survey.AskOne(&survey.Confirm{Message: message, Default: false}, &confirmed); err != nil {
		return err
	}
	if !confirmed {
		return fmt.Errorf("deletion cancelled")
	}
	return nil
}
