package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	iface "github.com/kamui-project/kamui-cli/internal/service/interface"
	"github.com/spf13/cobra"
)

func (d *DbCommand) addManagementCommands() {
	var project string
	check := func(cmd *cobra.Command, _ []string) error {
		if project == "" {
			return fmt.Errorf("--project is required (use kamui projects list)")
		}
		if format := resolveOutputFormat(cmd); format != "" && format != "text" && format != "json" {
			return fmt.Errorf("--output must be text or json")
		}
		return nil
	}
	bindProject := func(cmd *cobra.Command) {
		cmd.Flags().StringVarP(&project, "project", "p", "", "Project name or ID (required)")
	}
	list := &cobra.Command{Use: "list", Short: "List databases in a project", Args: cobra.NoArgs, PreRunE: check, RunE: func(cmd *cobra.Command, _ []string) error {
		result, err := d.Root().Container().DatabaseService().List(cmd.Context(), project)
		if err != nil {
			return err
		}
		if resolveOutputFormat(cmd) == "json" {
			if result == nil {
				result = []iface.DatabaseInfo{}
			}
			return databaseJSON(cmd, result)
		}
		rows := make([][]string, 0, len(result))
		for _, db := range result {
			rows = append(rows, databaseRow(db))
		}
		printTable(cmd.OutOrStdout(), "", []string{"ID", "NAME", "PROJECT", "SPEC", "STATUS"}, rows)
		if len(result) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No databases found. Create one with: kamui db create -p <project> --name <name>")
		}
		return nil
	}}
	get := &cobra.Command{Use: "get <database>", Short: "Show database metadata by name or ID", Long: "Show database metadata in the selected project. This command does not issue credentials or fetch a database password.", Args: cobra.ExactArgs(1), PreRunE: check, RunE: func(cmd *cobra.Command, args []string) error {
		result, err := d.Root().Container().DatabaseService().Get(cmd.Context(), project, args[0])
		if err != nil {
			return err
		}
		if resolveOutputFormat(cmd) == "json" {
			return databaseJSON(cmd, result)
		}
		printTable(cmd.OutOrStdout(), "", []string{"ID", "NAME", "PROJECT", "SPEC", "STATUS"}, [][]string{databaseRow(*result)})
		return nil
	}}
	var name, spec, routing string
	create := &cobra.Command{Use: "create", Short: "Request creation of a database", Long: "Create a database using the existing project permissions and billing rules. Provisioning is asynchronous; this command reports acceptance, not readiness. External connection credentials must be initialized separately in the dashboard.", Args: cobra.NoArgs, PreRunE: check, RunE: func(cmd *cobra.Command, _ []string) error {
		result, err := d.Root().Container().DatabaseService().Create(cmd.Context(), iface.CreateDatabaseInput{Project: project, Name: name, SpecType: spec, RoutingMode: routing})
		if err != nil {
			return err
		}
		if resolveOutputFormat(cmd) == "json" {
			return databaseJSON(cmd, result)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Database creation requested: %s (%s).\nProvisioning is asynchronous. Check with: kamui db get %s -p %s\n", storageText(result.Name), result.DatabaseID, result.DatabaseID, result.ProjectID)
		return nil
	}}
	create.Flags().StringVar(&name, "name", "", "Database name (1-30 characters; lowercase letters, digits, _ and -)")
	create.Flags().StringVar(&spec, "spec", "nano", "Database specification: nano, small, medium, large")
	create.Flags().StringVar(&routing, "routing-mode", "shared", "Database routing mode: shared or dedicated")
	_ = create.MarkFlagRequired("name")
	var yes bool
	remove := &cobra.Command{Use: "delete <database>", Short: "Permanently delete a database by name or ID", Long: "Permanently delete the database and its data in the selected project. The API refuses deletion while an app is using the database. Use --yes for non-interactive deletion.", Args: cobra.ExactArgs(1), PreRunE: check, RunE: func(cmd *cobra.Command, args []string) error {
		if !yes {
			if !isStdinTTY() {
				return fmt.Errorf("database deletion requires --yes when stdin is not a terminal")
			}
			confirmed := false
			message := "Permanently delete database " + storageText(args[0]) + " and all its data in project " + storageText(project) + "?"
			if err := survey.AskOne(&survey.Confirm{Message: message, Default: false}, &confirmed); err != nil {
				return err
			}
			if !confirmed {
				return fmt.Errorf("database deletion cancelled")
			}
		}
		if err := d.Root().Container().DatabaseService().Delete(cmd.Context(), project, args[0]); err != nil {
			return err
		}
		if resolveOutputFormat(cmd) == "json" {
			return databaseJSON(cmd, map[string]interface{}{"status": "deletion_requested", "database": args[0], "project": project})
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Database deletion requested.")
		return nil
	}}
	remove.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm permanent database deletion without prompting")
	for _, cmd := range []*cobra.Command{list, get, create, remove} {
		bindProject(cmd)
	}
	d.cmd.AddCommand(list, get, create, remove)
}

func databaseJSON(cmd *cobra.Command, value interface{}) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func databaseRow(db iface.DatabaseInfo) []string {
	return []string{db.ID, storageText(db.Name), storageText(db.ProjectName), storageText(db.SpecType), storageText(db.Status)}
}
