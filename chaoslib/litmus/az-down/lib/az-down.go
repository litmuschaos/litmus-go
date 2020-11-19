package lib

import "github.com/litmuschaos/litmus-go/pkg/log"

func AZDown() error {
	log.Info("YES")

	return nil
	// IF YOU WANT TO USE AN EXISTING CHAOSLIB THEN DELETE THIS FILE AND USE THE SAME
	// ELSE
	// ADD THE BUSINESS LOGIC OF THE ACTUAL CHAOS HERE
	// CALL THIS FUNCTION FROM <experiment-name.go>
}
