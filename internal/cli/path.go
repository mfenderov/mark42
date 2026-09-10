package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/mfenderov/mark42/internal/state"
)

var pathCmd = &cobra.Command{
	Use:   "path",
	Short: "Path and state utilities for harnesses and scripts",
}

var pathSlugCmd = &cobra.Command{
	Use:   "slug [directory]",
	Short: "Compute the project slug for a directory",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := ""
		if len(args) > 0 {
			dir = args[0]
		} else {
			var err error
			dir, err = os.Getwd()
			if err != nil {
				return err
			}
		}

		cmd.Println(state.ProjectSlug(dir))
		return nil
	},
}

var pathStateDirCmd = &cobra.Command{
	Use:   "state-dir [directory]",
	Short: "Compute the state directory path for a directory",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := ""
		if len(args) > 0 {
			dir = args[0]
		} else {
			var err error
			dir, err = os.Getwd()
			if err != nil {
				return err
			}
		}

		cmd.Println(state.Dir(dir))
		return nil
	},
}

func init() {
	pathCmd.AddCommand(pathSlugCmd)
	pathCmd.AddCommand(pathStateDirCmd)
	rootCmd.AddCommand(pathCmd)
}
