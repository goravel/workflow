// Package tests bootstraps the application for the feature test suites so the
// real facades are available.
package tests

import (
	"github.com/goravel/framework/testing"

	"goravel/bootstrap"
)

func init() {
	bootstrap.Boot()
}

// TestCase is embedded by the feature suites; the boot itself happens in init.
type TestCase struct {
	testing.TestCase
}
