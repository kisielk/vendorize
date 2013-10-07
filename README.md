vendorize
=========

vendorize is a tool for vendorizing go imports, including transitive dependencies

What does it do?
================
vendorize crawls the dependency graph of a given package and copies external dependencies
to a specified import prefix. It handles transitive dependencies, and updates the import
statements of all packages to point to the right place.

How do I use it?
================

First install vendorize using the standard `go get` command:

    $ go get github.com/kisielk/vendorize

Next, select a project whose dependencies you want to vendorize.
Select a package import path prefix where the dependencies will be copied.
These two paths make up the two mandatory positional arguments to vendorize.

Run the tool in "dry run" mode with the `-n` switch. This will give you a log of what *would*
happen, but does not actually make any changes to your package:

    $ vendorize -n github.com/kisiel/errcheck github.com/kisielk/errcheck/3rdparty
    2013/10/06 22:24:21 copying contents of "/Users/kamil/src/code.google.com/p/go.tools/go/exact" to "/Users/kamil/src/github.com/kisielk/errcheck/3rdparty/code.google.com/p/go.tools/go/exact"
    2013/10/06 22:24:21 copying contents of "/Users/kamil/src/code.google.com/p/go.tools/go/types" to "/Users/kamil/src/github.com/kisielk/errcheck/3rdparty/code.google.com/p/go.tools/go/types"
    2013/10/06 22:24:21 copying contents of "/Users/kamil/src/honnef.co/go/importer" to "/Users/kamil/src/github.com/kisielk/errcheck/3rdparty/honnef.co/go/importer"
    2013/10/06 22:24:21 copying contents of "/Users/kamil/src/github.com/kisielk/gotool" to "/Users/kamil/src/github.com/kisielk/errcheck/3rdparty/github.com/kisielk/gotool"

If you are satisfied, simply remove the `-n` switch to have vendorize copy the
dependencies and rewrite your package's import statements.

If you want to exclude some paths from being vendorized, specify the prefix
with the `-ignore` flag. The flag can be given multiple times to ignore multiple
prefixes.

In the example above, we trust the github.com/kisielk import prefix and the honnef.co/go/importer
package to remain stable, but I want to keep errcheck from breaking when there are API changes
in go.tools, a relatively unstable repository. We can ignore `github.com/kisielk` and
`honnef.co` to get the desired result:

    $ vendorize -n -ignore github.com/kisielk/ -ignore honnef.co/ github.com/kisiel/errcheck github.com/kisielk/errcheck/3rdparty
    2013/10/06 22:28:05 copying contents of "/Users/kamil/src/code.google.com/p/go.tools/go/exact" to "/Users/kamil/src/github.com/kisielk/errcheck/3rdparty/code.google.com/p/go.tools/go/exact"
    2013/10/06 22:28:05 copying contents of "/Users/kamil/src/code.google.com/p/go.tools/go/types" to "/Users/kamil/src/github.com/kisielk/errcheck/3rdparty/code.google.com/p/go.tools/go/types"

Once the `-n` flag flag is removed, the libraries will be copied to the given location and import statements
will be rewritten to point to their new import paths.
