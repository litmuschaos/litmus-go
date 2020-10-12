package main

import (
	"fmt"
	"log"

	cmd1 "github.com/litmuschaos/litmus-go/contribute/developer-guide"
	"github.com/spf13/cobra"
)

func main() {

	var filePath string

	var generate = &cobra.Command{
		Use:   "generate [flags]",
		Short: "Create a new custom experiment",
		Long:  "Create a new custom experiment",
		Args:  cobra.MinimumNArgs(1),
	}

	var experiment = &cobra.Command{
		Use:                   "experiment [flags]",
		Short:                 "Create a new custom experiment",
		Long:                  "Create a new custom experiment",
		Args:                  cobra.MaximumNArgs(0),
		DisableFlagsInUseLine: true,
		Run: func(cmd *cobra.Command, args []string) {
			if filePath == "" {
				log.Fatal("Error: must specify -f")
			}
			if err := cmd1.GenerateExperiment(&filePath, "experiment"); err != nil {
				log.Fatalf("error: %v", err)
			}
			fmt.Println("experiment created successfully")
		},
	}

	var chart = &cobra.Command{
		Use:                   "chart [flags]",
		Short:                 "Create the chart and experiment metadata",
		Long:                  "Create the chart and experiment metadata",
		Args:                  cobra.MaximumNArgs(0),
		DisableFlagsInUseLine: true,
		Run: func(cmd *cobra.Command, args []string) {
			if filePath == "" {
				log.Fatal("Error: must specify -f")
			}
			if err := cmd1.GenerateExperiment(&filePath, "chart"); err != nil {
				log.Fatalf("error: %v", err)
			}
			fmt.Println("chart created successfully")
		},
	}

	experiment.Flags().StringVarP(&filePath, "file", "f", "", "file detail")
	chart.Flags().StringVarP(&filePath, "file", "f", "", "file detail")

	var rootCmd = &cobra.Command{Use: "litmus-sdk"}
	generate.AddCommand(experiment)
	generate.AddCommand(chart)
	rootCmd.AddCommand(generate)
	rootCmd.Execute()
}
