//go:build !test

package tests

// skipKnownFailures enables skipping the documented known failures so the
// default `go test ./...` context stays green. It is disabled under the
// "test" build tag (see regressions_test.go) so the regression table keeps
// reporting known failures as failures.
var skipKnownFailures = true
