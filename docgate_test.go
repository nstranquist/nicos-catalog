package catalog

import (
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
	"testing"
)

// configStructs are the types a host authors rather than reads. Every exported
// field on these must document what a caller is choosing, because an
// unexplained field here becomes a silent misconfiguration.
var configStructs = map[string]bool{
	"Layout":             true,
	"ProjectionPolicy":   true,
	"SearchOptions":      true,
	"Limits":             true,
	"StaticProvider":     true,
	"FilesystemProvider": true,
	"Entity":             true,
	"Ref":                true,
	"Record":             true,
}

// TestEveryExportedSymbolIsDocumented keeps pkg.go.dev honest.
//
// There is no allowlist. A new exported symbol is part of the published API the
// moment it merges, and an undocumented one renders as a bare signature to
// every reader who is deciding whether to depend on this module.
func TestEveryExportedSymbolIsDocumented(t *testing.T) {
	fset := token.NewFileSet()
	packages, err := parser.ParseDir(fset, ".", func(info fs.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	pkg, ok := packages["catalog"]
	if !ok {
		t.Fatal("package catalog not found in the current directory")
	}
	// Mode 0 restricts the report to exported declarations. Unexported helpers
	// follow the surrounding comment density; they are not published API.
	docs := doc.New(pkg, "github.com/nstranquist/nicos-catalog", 0)

	if strings.TrimSpace(docs.Doc) == "" {
		t.Fatal("the package has no doc comment; pkg.go.dev would render a blank synopsis")
	}

	report := func(kind, name string) {
		t.Errorf("%s %s has no doc comment", kind, name)
	}
	for _, value := range docs.Consts {
		if strings.TrimSpace(value.Doc) == "" {
			report("const", strings.Join(value.Names, ", "))
		}
	}
	for _, value := range docs.Vars {
		if strings.TrimSpace(value.Doc) == "" {
			report("var", strings.Join(value.Names, ", "))
		}
	}
	for _, fn := range docs.Funcs {
		if strings.TrimSpace(fn.Doc) == "" {
			report("func", fn.Name)
		}
	}
	for _, typ := range docs.Types {
		if strings.TrimSpace(typ.Doc) == "" {
			report("type", typ.Name)
		}
		for _, fn := range typ.Funcs {
			if strings.TrimSpace(fn.Doc) == "" {
				report("func", fn.Name)
			}
		}
		for _, method := range typ.Methods {
			if strings.TrimSpace(method.Doc) == "" {
				report("method", typ.Name+"."+method.Name)
			}
		}
		for _, value := range typ.Consts {
			if strings.TrimSpace(value.Doc) == "" {
				report("const", strings.Join(value.Names, ", "))
			}
		}
		// Field-level docs are required on the structs a caller fills in
		// themselves, where a wrong value is a silent misconfiguration rather
		// than a compile error. Result and report structs are exempt: their
		// fields are read, not authored, and "OK reports whether it is OK"
		// is noise rather than documentation.
		if configStructs[typ.Name] {
			assertStructFieldsDocumented(t, typ)
		}
	}
}

func assertStructFieldsDocumented(t *testing.T, typ *doc.Type) {
	t.Helper()
	for _, spec := range typ.Decl.Specs {
		typeSpec, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok || structType.Fields == nil {
			continue
		}
		for _, field := range structType.Fields.List {
			for _, name := range field.Names {
				if !name.IsExported() {
					continue
				}
				if strings.TrimSpace(field.Doc.Text()) == "" && strings.TrimSpace(field.Comment.Text()) == "" {
					t.Errorf("field %s.%s has no doc comment", typeSpec.Name.Name, name.Name)
				}
			}
		}
	}
}
