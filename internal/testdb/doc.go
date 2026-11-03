// Package testdb gives each integration-test package its own throwaway
// Postgres. Its code lives behind the "integration" build tag, so neither
// testcontainers-go nor a Docker client is compiled into a normal build or
// unit-test run — only `go test -tags=integration` pulls it in.
//
// Usage, in one *_integration_test.go file per package:
//
//	func TestMain(m *testing.M) { os.Exit(testdb.Run(m)) }
//
// and testdb.DSN() wherever a test opens a connection.
package testdb
