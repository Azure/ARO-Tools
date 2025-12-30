package client

import (
	"errors"
	"fmt"
	"time"
)

type NoDeploymentsFoundError struct {
	Environment    Environment
	Since          time.Time
	Until          time.Time
	Revision       string
	SourceRevision string
}

func (e *NoDeploymentsFoundError) Error() string {
	return fmt.Sprintf(
		"no deployments found in '%s' between %s and %s matching the specified revision filters",
		e.Environment,
		e.Since,
		e.Until,
	)
}

func IsNoDeploymentsFound(err error) bool {
	var e *NoDeploymentsFoundError
	return errors.As(err, &e)
}
