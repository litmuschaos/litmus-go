package main

import (
	"fmt"
	"log"

	sdkCmd "github.com/litmuschaos/litmus-go/contribute/developer-guide"
	"github.com/spf13/cobra"
)

func main() {

	var (
		filePath  string
		chartType string
	)

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
		Example:               "./litmus-sdk generate experiment -f=attribute.yaml",
		DisableFlagsInUseLine: true,
		Run: func(cmd *cobra.Command, args []string) {
			if err := sdkCmd.GenerateExperiment(filePath, chartType, "experiment"); err != nil {
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
		Example:               "./litmus-sdk generate chart -f=attribute.yaml",
		DisableFlagsInUseLine: true,
		Run: func(cmd *cobra.Command, args []string) {
			if err := sdkCmd.GenerateExperiment(filePath, chartType, "chart"); err != nil {
				log.Fatalf("error: %v", err)
			}
			fmt.Println("chart created successfully")
		},
	}

	experiment.Flags().StringVarP(&filePath, "file", "f", "", "path of the attribute.yaml manifest")
	chart.Flags().StringVarP(&filePath, "file", "f", "", "path of the attribute.yaml manifest")
	chart.Flags().StringVarP(&chartType, "type", "t", "all", "type of the chaos chart")
	chart.MarkFlagRequired("file")
	experiment.MarkFlagRequired("file")

	var rootCmd = &cobra.Command{Use: "litmus-sdk"}
	generate.AddCommand(experiment)
	generate.AddCommand(chart)
	rootCmd.AddCommand(generate)
	rootCmd.Execute()
}
