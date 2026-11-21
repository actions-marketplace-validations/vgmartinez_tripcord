package rules

import (
	"slices"
	"strings"
)

// fixtureDirs are the directory names that hold test input: a harness
// loads its config from the project root, not from a copy kept under one
// of these for a test to read.
var fixtureDirs = []string{"fixtures", "__fixtures__", "testdata", "test-fixtures", "test_fixtures", "tests", "test"}

// FixturePath reports whether rel, a repository-relative slash path, is
// under a test-fixture or test directory: any directory component, not
// the file name, is exactly one of fixtureDirs. A name that only contains
// one of them (contests/, latest/, testing-guide/) does not count, and
// neither does a different case (Fixtures/). Rules read it as
// config.fixture_path; it is the same set of names TC-CFG-006 and
// TC-CFG-010 match in their own clauses.
func FixturePath(rel string) bool {
	parts := strings.Split(rel, "/")
	return slices.ContainsFunc(parts[:len(parts)-1], func(p string) bool {
		return slices.Contains(fixtureDirs, p)
	})
}
