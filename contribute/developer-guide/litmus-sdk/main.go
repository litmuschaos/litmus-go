package main

import (
	"fmt"

	cmd1 "github.com/litmuschaos/litmus-go/contribute/developer-guide"
	"github.com/spf13/cobra"
)

func main() {

	var filePath string
	var Type string

	var create = &cobra.Command{
		Use:   "create [flags]",
		Short: "Create a new custom experiment",
		Long:  "Create a new custom experiment",
		Args:  cobra.MaximumNArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("args: %v, type: %v, filePath: %v\n", args, Type, filePath)
			cmd1.GenerateExperiment(&filePath, &Type)
			fmt.Println("created successfully")
		},
	}

	create.Flags().StringVarP(&Type, "type", "t", "experiment", "experiment type")
	create.Flags().StringVarP(&filePath, "file", "f", "", "file detail")

	var rootCmd = &cobra.Command{Use: "litmus-sdk"}
	rootCmd.AddCommand(create)
	rootCmd.Execute()
}
