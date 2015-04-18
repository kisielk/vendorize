package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

type chooseGOPATHTestCase struct {
	dirs    []string // relative to temp dir
	gopaths []string // relative to temp dir
	dest    string
	want    string
}

func TestChooseGOPATH(t *testing.T) {
	for _, tc := range []chooseGOPATHTestCase{
		// pick the only available gopath
		{nil, []string{"p1"}, "github.com/user/proj1", "p1"},
		// pick the first of several when none match
		{nil, []string{"p1", "p2"}, "github.com/user/proj1", "p1"},
		// pick the one that exists
		{[]string{"p2"}, []string{"p1", "p2"}, "github.com/user/proj1", "p2"},
		// pick the first when both exist
		{[]string{"p1", "p2"}, []string{"p1", "p2"}, "github.com/user/proj1", "p1"},
		// pick a path with matching prefix
		{[]string{"p1", "p2/src/github.com"}, []string{"p1", "p2"}, "github.com/user/proj1", "p2"},
		// pick a path with better matching prefix
		{
			[]string{"p1/src/github.com", "p2/src/github.com/user"},
			[]string{"p1", "p2"},
			"github.com/user/proj1",
			"p2",
		},
		// break ties toward the front
		{
			[]string{"p1/src/github.com/user/proj1", "p2/src/github.com/user/proj1"},
			[]string{"p1", "p2"},
			"github.com/user/proj1",
			"p1",
		},
	} {
		testChooseGOPATH(t, tc)
	}
}

func testChooseGOPATH(t *testing.T, tc chooseGOPATHTestCase) {
	tempdir, err := ioutil.TempDir("", "vendorize-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempdir)

	tempdir, err = filepath.Abs(tempdir)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range tc.dirs {
		if err := os.MkdirAll(filepath.Join(tempdir, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	fullGOPATHs := make([]string, len(tc.gopaths))
	for i, gopath := range tc.gopaths {
		fullGOPATHs[i] = filepath.Join(tempdir, gopath)
	}
	got := chooseGOPATH(fullGOPATHs, tc.dest)
	want := filepath.Join(tempdir, tc.want)
	if got != want {
		t.Errorf("for test case %#v: got: %q; want: %q", tc, got, want)
	}
}
