package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	fake     bool
	rewrites map[string]string // rewrites that have been performed
	visited  map[string]bool   // packages that have already been visited
	gopath   string            // the last component of GOPATH
)

func main() {
	flag.BoolVar(&fake, "n", false, "If true, don't actually do anything")
	flag.BoolVar(&verbose, "v", false, "Provide verbose output")
	flag.Var(&ignorePrefixes, "ignore", "Package prefix to ignore. Can be given multiple times.")
	flag.Parse()

	gopaths := filepath.SplitList(os.Getenv("GOPATH"))
	gopath = gopaths[len(gopaths)-1]
	if gopath == "" {
		log.Fatal("GOPATH must be set")
	}
	pkgName := flag.Arg(0)
	if pkgName == "" {
		log.Fatal("need a package name")
	}
	dest := flag.Arg(1)
	if dest == "" {
		log.Fatal("need a destination path")
	}

	ignorePrefixes = append(ignorePrefixes, pkgName)
	ignorePrefixes = append(ignorePrefixes, dest)
	rewrites = make(map[string]string)
	visited = make(map[string]bool)

	err := vendorize(pkgName, dest)
	if err != nil {
		log.Fatal(err)
	}
}

func vendorize(path, dest string) error {
	if visited[path] {
		return nil
	}
	visited[path] = true

	verbosef("vendorizing %s", path)
	rootPkg, err := buildPackage(path)
	if err != nil {
		return fmt.Errorf("couldn't import %s: %s", path, err)
	}
	if rootPkg.Goroot {
		return fmt.Errorf("can't vendorize packages from GOROOT")
	}

	allImports := getAllImports(rootPkg)

	var pkgs []*build.Package
	for _, imp := range allImports {
		if imp == "C" {
			continue
		}
		pkg, err := buildPackage(imp)
		if err != nil {
			return fmt.Errorf("%s: couldn't import %s: %s", path, imp, err)
		}
		if !pkg.Goroot {
			pkgs = append(pkgs, pkg)
		}
	}

	for _, pkg := range pkgs {
		err := vendorize(pkg.ImportPath, dest)
		if err != nil {
			return fmt.Errorf("couldn't vendorize %s: %s", pkg.ImportPath, err)
		}
	}

	pkgDir := rootPkg.Dir

	if !ignored(path) {
		newPath := dest + "/" + path
		pkgDir = filepath.Join(gopath, "src", newPath)
		err = copyDir(pkgDir, rootPkg.Dir)
		if err != nil {
			return fmt.Errorf("couldn't copy %s: %s", path, err)
		}
		rewrites[path] = newPath
	}

	// Rewrite any import lines in the package.
	for _, files := range [][]string{
		rootPkg.GoFiles, rootPkg.CgoFiles, rootPkg.TestGoFiles, rootPkg.XTestGoFiles,
	} {
		for _, file := range files {
			if len(rewrites) > 0 {
				destFile := filepath.Join(pkgDir, file)
				verbosef("rewriting imports in %q", destFile)
				err := rewriteFile(destFile, filepath.Join(rootPkg.Dir, file), rewrites)
				if err != nil {
					return fmt.Errorf("%s: couldn't rewrite file %q: %s", path, file, err)
				}
			}
		}
	}
	return nil
}

// package prefixes that should not be copied
var ignorePrefixes stringSliceFlag

func ignored(path string) bool {
	_, rewritten := rewrites[path]
	if rewritten {
		return true
	}
	for _, prefix := range ignorePrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// copyFile copies the file given by src to dest, creating dest with the permissions given by perm.
func copyFile(dest, src string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dest, os.O_RDWR|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// copyDir non-recursively copies the contents of the src directory to dest.
func copyDir(dest, src string) error {
	log.Printf("copying contents of %q to %q", src, dest)
	if !fake {
		err := os.MkdirAll(dest, 0770)
		if err != nil {
			return fmt.Errorf("couldn't make destination directory", dest)
		}
	}

	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// We don't recurse.
		if info.IsDir() {
			if path != src {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		destFile := filepath.Join(dest, relPath)
		verbosef("copying %q to %q", path, destFile)
		if fake {
			return nil
		}
		return copyFile(destFile, path, info.Mode().Perm())
	})
}

// getAllImports returns a list of all import paths in the Go files of pkg.
func getAllImports(pkg *build.Package) []string {
	allImports := make(map[string]bool)
	for _, imports := range [][]string{pkg.Imports, pkg.TestImports, pkg.XTestImports} {
		for _, imp := range imports {
			allImports[imp] = true
		}
	}
	result := make([]string, 0, len(allImports))
	for imp := range allImports {
		result = append(result, imp)
	}
	return result
}

// builtPackages maintains a cache of package builds.
var builtPackages map[string]*build.Package

// buildPackage builds a package given by the path.
func buildPackage(path string) (*build.Package, error) {
	if builtPackages == nil {
		builtPackages = make(map[string]*build.Package)
	}
	if pkg, ok := builtPackages[path]; ok {
		return pkg, nil
	}

	ctx := build.Default
	// TODO(kisielk): support relative imports?
	pkg, err := ctx.Import(path, "", 0)
	if err != nil {
		return nil, err
	}
	builtPackages[path] = pkg
	return pkg, nil
}

func rewriteFile(dest, path string, m map[string]string) error {
	if fake {
		return nil
	}

	f, err := ioutil.TempFile("", "vendorize")
	if err != nil {
		return err
	}
	defer f.Close()
	err = rewriteFileImports(path, m, f)
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), dest)
}

func rewriteFileImports(path string, m map[string]string, w io.Writer) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return err
	}

	removeImportComment(fset, f)

	for _, s := range f.Imports {
		path, err := strconv.Unquote(s.Path.Value)
		if err != nil {
			panic(err)
		}
		if replacement, ok := m[path]; ok {
			s.Path.Value = strconv.Quote(replacement)
		}
	}

	return format.Node(w, fset, f)
}

// removeImportComment removes the import comment in f, if one exists.
func removeImportComment(fset *token.FileSet, f *ast.File) {
	// The import comment must be immediately after (and on the same line as)
	// the package declaration.
	// Loop through the comments until we find one that is on that line or beyond.
	nameLine := fset.Position(f.Name.NamePos).Line
	for _, comments := range f.Comments {
		commentLine := fset.Position(comments.Pos()).Line
		if commentLine == nameLine {
			comment := comments.List[0]
			if strings.HasPrefix(comment.Text, "// import ") ||
				strings.HasPrefix(comment.Text, "/* import ") {
				comment.Text = "// #vendored#"
			}
			return
		}
		if commentLine > nameLine {
			return
		}
	}
}

// stringSliceFlag is a flag.Value that accumulates multiple flags in to a slice.
type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	return fmt.Sprintf("%v", []string(*s))
}

func (s *stringSliceFlag) Set(value string) error {
	*s = append(*s, value)
	return nil
}

// verbose controls the level of logging.
var verbose bool

// verbosef logs only if verbose is true.
func verbosef(s string, args ...interface{}) {
	if verbose {
		log.Printf(s, args...)
	}
}
