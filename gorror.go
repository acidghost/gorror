// (c) Copyright 2021, Gorror Authors.
//
// Licensed under the terms of the GNU GPL License version 3.

// Gorror is a tool to generate error structures starting from a template specification.
// Given the name of a string type T, Gorror will use all the constants defined with type T to
// generate Go source code for types implementing error (and more).
package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/tools/go/packages"
)

var (
	flagTyp    = flag.String("type", "", "type of the error specifications; required")
	flagOut    = flag.String("output", "", "output file name; default srcdir/<type>_def.go")
	flagIs     = flag.Bool("is", false, "enable compatibility with errors.Is")
	flagPub    = flag.Bool("P", false, "generate public errors")
	flagSuffix = flag.String("suffix", "", "to drop from the end of the error specs")
	flagImps   = flag.String("import", "", "comma-separated list of imports")
)

//go:embed banner.txt
var banner string

//go:embed VERSION
var version string

var tmplRE = regexp.MustCompile(`{{([A-Za-z0-9_\.\[\]]+) (\*?[A-Za-z0-9_\.]+) (%[A-Za-z0-9#\.\+]+)}}`)

func Usage() {
	fmt.Fprintf(os.Stderr, "\n%s\nVer: %s\n\n", banner, version)
	fmt.Fprintf(os.Stderr, "Usage of Gorror:\n")
	fmt.Fprintf(os.Stderr, "\tgorror [flags] -type T [directory]\n")
	fmt.Fprintf(os.Stderr, "\tgorror [flags] -type T files... # Must be a single package\n")
	fmt.Fprintf(os.Stderr, "Flags:\n")
	flag.PrintDefaults()
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("gorror: ")
	flag.Usage = Usage
	flag.Parse()

	if *flagTyp == "" {
		flag.Usage()
		os.Exit(1)
	}

	args := flag.Args()
	if len(args) < 1 {
		args = []string{"."}
	}

	var dir string
	if len(args) == 1 && isDirectory(args[0]) {
		dir = args[0]
	} else {
		dir = filepath.Dir(args[0])
	}

	imports := make([]string, 0)
	for _, s := range strings.Split(*flagImps, ",") {
		s = strings.TrimSpace(s)
		if len(s) > 0 {
			imports = append(imports, s)
		}
	}
	sort.Strings(imports)

	g := Generator{
		typeName:   *flagTyp,
		compatIs:   *flagIs,
		makePub:    *flagPub,
		specSuffix: *flagSuffix,
		imports:    imports,
	}

	g.loadPackage(args)

	if len(g.specs) < 1 {
		log.Printf("no errors of type %s found", g.typeName)
		return
	}

	g.header()
	for _, err := range g.specs {
		g.generate(err)
	}

	src := g.format()

	// Write to file.
	outputName := *flagOut
	if outputName == "" {
		baseName := fmt.Sprintf("%s_def.go", g.typeName)
		outputName = filepath.Join(dir, strings.ToLower(baseName))
	}
	err := os.WriteFile(outputName, src, 0644)
	if err != nil {
		log.Fatalf("writing output: %s", err)
	}
}

func isDirectory(s string) bool {
	stat, err := os.Stat(s)
	if err != nil {
		log.Fatal(err)
	}
	return stat.IsDir()
}

type Generator struct {
	typeName   string
	compatIs   bool
	makePub    bool
	specSuffix string
	imports    []string
	buf        bytes.Buffer
	specs      []ErrorSpec
	pkgName    string
}

// ErrorSpec represents an error to be generated. The two fields correspond to the constant
// declaration name and the template in the associated string value.
type ErrorSpec struct{ name, template string }

// loadPackage loads the (expected) single package given a pattern and inspects
// the source code files to collect error definitions.
func (g *Generator) loadPackage(pattern []string) {
	cfg := &packages.Config{
		Mode:  packages.NeedSyntax,
		Tests: false,
	}
	pkgs, err := packages.Load(cfg, pattern...)
	if err != nil {
		log.Fatal(err)
	}
	if len(pkgs) != 1 {
		log.Fatalf("too many packages: found %d, expected 1", len(pkgs))
	}
	pkg := pkgs[0]
	for _, file := range pkg.Syntax {
		g.processFile(file)
		g.pkgName = file.Name.Name
		ast.Inspect(file, g.processFile)
	}
}

// Printf is an utility to append data to the internal buffer.
func (g *Generator) Printf(fmtStr string, args ...interface{}) {
	fmt.Fprintf(&g.buf, fmtStr, args...)
}

// processFile is called by ast.Inspect and take care of collecting the error definitions.
func (g *Generator) processFile(node ast.Node) bool {
	decl, ok := node.(*ast.GenDecl)
	if !ok || decl.Tok != token.CONST {
		return true
	}
	for _, spec := range decl.Specs {
		vspec := spec.(*ast.ValueSpec) // Guaranteed to succeed as this is CONST.
		var typ string
		if vspec.Type == nil {
			ce, ok := vspec.Values[0].(*ast.CallExpr)
			if !ok {
				continue
			}
			f, ok := ce.Fun.(*ast.Ident)
			if !ok {
				continue
			}
			typ = f.Name
		} else {
			ident, ok := vspec.Type.(*ast.Ident)
			if !ok {
				continue
			}
			typ = ident.Name
		}
		if typ != g.typeName {
			continue
		}
		name := vspec.Names[0].Name
		var template string
		switch v := vspec.Values[0].(type) {
		case *ast.CallExpr:
			s, ok := v.Args[0].(*ast.BasicLit)
			if !ok || s.Kind != token.STRING {
				log.Fatalf("expected string literal, got %#v\n", v.Args[0])
			}
			template = s.Value
		case *ast.BasicLit:
			if v.Kind != token.STRING {
				log.Fatalf("expected string literal or cast to %s, got %#v\n", typ, v)
			}
			template = v.Value
		default:
			log.Fatalf("expected string literal or cast to %s, got %#v\n", typ, v)
		}
		template, err := strconv.Unquote(template)
		if err != nil {
			log.Fatal(err)
		}
		g.specs = append(g.specs, ErrorSpec{name, template})
	}
	return false
}

// header generates the package header, imports and common types.
func (g *Generator) header() {
	// Generate header and package declaration.
	g.Printf("// Errors generated by Gorror; DO NOT EDIT.\n\npackage %s\n\n", g.pkgName)
	// Generate import statements.
	imports := make([]string, 0, len(g.imports)+2)
	imports = append(g.imports, "fmt", "errors")
	sort.Strings(imports)
	g.Printf("import (\n")
	for _, imp := range imports {
		g.Printf("\t%q\n", imp)
	}
	g.Printf(")\n\n")
	// Generate _errWrap structure.
	g.Printf("type _errWrap struct{ cause error }\n")
	g.Printf("func (w *_errWrap) Unwrap() error { return w.cause }\n\n")

	if g.compatIs {
		g.Printf("func (%s) Error() string { panic(\"Should not be called\") }\n\n", g.typeName)
	} else {
		g.Printf(`func (e %[1]s) IsIn(err error) bool {
	var ei interface { Is(%[1]s) bool; Unwrap() error }
	if errors.As(err, &ei) {
		if ei.Is(e) { return true }
		return e.IsIn(ei.Unwrap())
	}
	return false}`, g.typeName)
		g.Printf("\n\n")
	}
}

// generate generates the code for a single error implementations.
func (g *Generator) generate(spec ErrorSpec) {
	structName := g.structName(spec.name)
	template := parseTemplate(spec.template)

	// Generate structure for error.
	g.Printf("type %s struct {\n", structName)
	if template.wrap != NoWrap {
		g.Printf("\t_errWrap\n")
	}
	for _, f := range template.fields {
		g.Printf("\t%s %s\n", f.name, f.typ)
	}
	g.Printf("}\n\n")

	// Generate constructor with all arguments.
	constPrefix := "new"
	if g.makePub {
		constPrefix = "New"
	}
	g.Printf("func %s%s(", constPrefix, strings.Title(structName))
	for i, f := range template.fields {
		g.Printf("%s %s", f.name, f.typ)
		if i < len(template.fields)-1 {
			g.Printf(", ")
		}
	}
	if template.wrap == MustWrap {
		if len(template.fields) > 0 {
			g.Printf(", ")
		}
		g.Printf("err error")
	}
	g.Printf(") *%[1]s {\n\treturn &%[1]s{", structName)
	if template.wrap == MustWrap || template.wrap == OptWrap {
		ew := "_errWrap{nil}"
		if template.wrap == MustWrap {
			ew = "_errWrap{err}"
		}
		g.Printf(ew)
		if len(template.fields) > 0 {
			g.Printf(", ")
		}
	}
	for i, f := range template.fields {
		g.Printf("%s", f.name)
		if i < len(template.fields)-1 {
			g.Printf(", ")
		}
	}
	g.Printf("}\n}\n\n")

	// Generate Error method.
	g.Printf("func (e *%s) Error() string {\n", structName)
	switch template.wrap {
	case OptWrap:
		g.Printf("\tif e.cause == nil {\n\t\treturn fmt.Sprintf(\"%v\"", template.fmt)
		// Add call to Sprintf w/o cause.
		for _, f := range template.fields {
			g.Printf(", e.%s", f.val)
		}
		g.Printf(")\n\t}\n\treturn fmt.Sprintf(\"%s: %%v\", ", template.fmt)
		// Add params to Sprintf w/ cause.
		for _, f := range template.fields {
			g.Printf("e.%s, ", f.val)
		}
		g.Printf("e.cause)\n")
	case NoWrap:
		g.Printf("\treturn fmt.Sprintf(\"%v\"", template.fmt)
		for _, f := range template.fields {
			g.Printf(", e.%s", f.val)
		}
		g.Printf(")\n")
	case MustWrap:
		g.Printf("\treturn fmt.Sprintf(\"%s: %%v\", ", template.fmt)
		// Add params to Sprintf w/ cause.
		for _, f := range template.fields {
			g.Printf("e.%s, ", f.val)
		}
		g.Printf("e.cause)\n")
	}
	g.Printf("}\n")

	if template.wrap != NoWrap {
		// Generate Wrap method.
		g.Printf(`
func (e *%s) Wrap(cause error) error {
	e.cause = cause
	return e
}
`, structName)
	}

	// Generate Is method.
	if g.compatIs {
		g.Printf("\nfunc (*%s) Is(e error) bool { return e == %s }\n\n", structName, spec.name)
	} else {
		g.Printf("\nfunc (*%s) Is(e %s) bool { return e == %s }\n\n", structName, g.typeName, spec.name)
	}
}

func (g *Generator) structName(specName string) string {
	var b strings.Builder
	runes := []rune(specName)
	if g.makePub {
		b.WriteRune(unicode.ToUpper(runes[0]))
	} else {
		b.WriteRune(unicode.ToLower(runes[0]))
	}
	rest := string(runes[1:])
	if len(g.specSuffix) > 0 {
		rest = strings.TrimSuffix(rest, g.specSuffix)
	}
	b.WriteString(rest)
	return b.String()
}

type ParsedTemplate struct {
	wrap   WrapMode
	fields []Field
	fmt    string
}

type WrapMode int

const (
	OptWrap WrapMode = iota
	NoWrap
	MustWrap
)

// Field represents a field from a parsed template.
type Field struct {
	name string // name of the field
	typ  string // type of the field
	fmt  string // format verb for the field
	val  string // accessor to use when formatting (e.g. name.Field)
}

func parseTemplate(template string) ParsedTemplate {
	wrap := OptWrap
	switch {
	case strings.HasPrefix(template, "wrap:"):
		wrap = MustWrap
		template = strings.TrimPrefix(template, "wrap:")
	case strings.HasPrefix(template, "nowrap:"):
		wrap = NoWrap
		template = strings.TrimPrefix(template, "nowrap:")
	}
	matches := tmplRE.FindAllStringSubmatch(template, -1)
	fields := make([]Field, 0, len(matches))
	tmplStr := template
	for _, match := range matches {
		fExpr, fType, fFmt := match[1], match[2], match[3]
		nameAST, err := parser.ParseExpr(fExpr)
		if err != nil {
			log.Fatal(err)
		}
		fNameIdent := findExprRoot(nameAST)
		if fNameIdent == nil {
			log.Fatalf("Could not find root node of expression %q", fExpr)
		}
		tmplStr = strings.Replace(tmplStr, match[0], fFmt, 1)
		fields = append(fields, Field{fNameIdent.Name, fType, fFmt, fExpr})
	}
	return ParsedTemplate{wrap, fields, tmplStr}
}

func findExprRoot(node ast.Expr) *ast.Ident {
	for {
		switch n := node.(type) {
		case *ast.SelectorExpr:
			node = n.X
		case *ast.IndexExpr:
			node = n.X
		case *ast.Ident:
			return n
		default:
			return nil
		}
	}
}

func (g *Generator) format() []byte {
	src, err := format.Source(g.buf.Bytes())
	if err != nil {
		log.Printf("warning: failed to format generated code: %v\n", err)
		log.Printf("warning: try to compile the output to check the error\n")
		if len(src) == 0 {
			log.Fatalf("format produced empty output\n%s\n", g.buf.String())
		}
	}
	return src
}
