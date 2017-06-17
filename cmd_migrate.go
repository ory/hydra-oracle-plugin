package main

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

// migrateCmd represents the migrate command
var migrateCmd = &cobra.Command{
	Use: "migrate <oracle-url>",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) != 1 {
			fmt.Println(cmd.UsageString())
			return
		}

		if db, err := Connect(args[0]); err != nil {
			log.Fatalf("Could not connect to database because: %s", err)
		} else if err = CreateSchemas(db); err != nil {
			log.Fatalf("Could not create schemas because: %s", err)
		}
	},
}

func init() {
	RootCmd.AddCommand(migrateCmd)
}
