package main

import (
	"flag"
	"fmt"
	"go/build"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var fake = flag.Bool("n", true, "If true, don't actually do anything")

func main() {
	flag.Parse()
	pkgName := flag.Arg(0)
	if pkgName == "" {
		log.Fatal("need a package name")
	}
	dest := flag.Arg(1)
	if dest == "" {
		log.Fatal("need a destination path")
	}

	err := vendorize(pkgName, dest)
	if err != nil {
		log.Fatal(err)
	}
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

func vendorize(path, dest string) error {
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
		if strings.HasPrefix(imp, path) || strings.HasPrefix(imp, dest) {
			// don't process things we presumably don't need to vendor
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

	destImportPath := dest + "/" + path
	rewrites := make(map[string]string)
	for _, pkg := range pkgs {
		err := vendorize(pkg.ImportPath, dest)
		if err != nil {
			return fmt.Errorf("couldn't vendorize %s: %s", pkg.ImportPath, err)
		}
		rewrites[pkg.ImportPath] = destImportPath
	}

	gopaths := filepath.SplitList(os.Getenv("GOPATH"))
	gopath := gopaths[len(gopaths)-1]

	destDir := filepath.Join(gopath, "src", destImportPath)

	// Copy all the files to the destination.
	err = filepath.Walk(rootPkg.Dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// We don't recurse.
		if path != rootPkg.Dir && info.IsDir() {
			return filepath.SkipDir
		}

		src := filepath.Join(rootPkg.Dir, path)
		dest := filepath.Join(destDir, path)
		log.Printf("copying %q to %q", src, dest)
		if *fake {
			return nil
		}
		return copyFile(dest, src, info.Mode().Perm())
	})
	if err != nil {
		return fmt.Errorf("couldn't copy %s: %s", rootPkg, err)
	}

	// Rewrite any imports
	for _, files := range [][]string{
		rootPkg.GoFiles, rootPkg.CgoFiles, rootPkg.TestGoFiles, rootPkg.XTestGoFiles,
	} {
		for _, file := range files {
			if len(rewrites) > 0 {
				destFile := filepath.Join(gopath, "src", destImportPath, file)
				err := rewriteFile(destFile, filepath.Join(rootPkg.Dir, file), rewrites)
				if err != nil {
					return fmt.Errorf("%s: couldn't rewrite file %q: %s", path, file, err)
				}
			}
		}
	}
	return nil
}

// builtPackages keeps a cache of package builds.
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
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	return rewriteImports(path, m, f)
}

func rewriteImports(path string, m map[string]string, w io.Writer) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return err
	}

	for _, s := range f.Imports {
		path, err := strconv.Unquote(s.Path.Value)
		if err != nil {
			panic(err)
		}
		if replacement, ok := m[path]; ok {
			s.Path.Value = strconv.Quote(replacement)
		}
	}

	return printer.Fprint(w, fset, f)
}
