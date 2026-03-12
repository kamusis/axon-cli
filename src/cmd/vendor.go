package cmd

import "github.com/spf13/cobra"

var vendorCmd = &cobra.Command{
	Use:   "vendor",
	Short: "Manage external vendor content synced into the Hub",
	Long: `The vendor command family manages external repo/subdir content
that is mirrored as plain files into the Hub.

Vendored content is defined in the 'vendors' block of ~/.axon/axon.yaml.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(vendorCmd)
}
