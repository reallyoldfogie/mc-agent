
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/types"
	"log"
	"os"
	"strings"

	"golang.org/x/tools/go/packages"
)

var (
	typeName = flag.String("type", "", "Interface type name (e.g. models.Action)")
	outFile  = flag.String("out", "iface.json", "Output file")
)

type AgentSpec struct {
	Interface string     `json:"interface"`
	Tools     []ToolSpec `json:"tools"`
}

type ToolSpec struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Parameters  map[string]string `json:"parameters"`
	Returns     map[string]string `json:"returns"`
}

func main() {
	flag.Parse()

	if *typeName == "" {
		log.Fatal("-type is required")
	}

	cfg := &packages.Config{
		Mode: packages.NeedTypes |
			packages.NeedSyntax |
			packages.NeedTypesInfo |
			packages.NeedName,
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		log.Fatal(err)
	}

	iface, ifaceObj, pkg := findInterface(pkgs, *typeName)
	if iface == nil {
		log.Fatalf("interface %s not found", *typeName)
	}

	methodDocs := extractDocs(pkg, ifaceObj.Name())

	spec := AgentSpec{
		Interface: ifaceObj.Name(),
	}

	for i := 0; i < iface.NumMethods(); i++ {
		m := iface.Method(i)
		sig := m.Type().(*types.Signature)

		tool := ToolSpec{
			Name:        m.Name(),
			Description: methodDocs[m.Name()],
			Parameters:  map[string]string{},
			Returns:     map[string]string{},
		}

		for i := 0; i < sig.Params().Len(); i++ {
			p := sig.Params().At(i)
			tool.Parameters[p.Name()] = typeString(p.Type())
		}

		for i := 0; i < sig.Results().Len(); i++ {
			r := sig.Results().At(i)
			tool.Returns[r.Name()] = typeString(r.Type())
		}

		spec.Tools = append(spec.Tools, tool)
	}

	data, _ := json.MarshalIndent(spec, "", "  ")
	os.WriteFile(*outFile, data, 0644)

	fmt.Println("Generated", *outFile)
}

func findInterface(pkgs []*packages.Package, fullType string) (*types.Interface, *types.TypeName, *packages.Package) {
	var pkgPath, typeName string

	if idx := strings.LastIndex(fullType, "."); idx != -1 {
		pkgPath = fullType[:idx]
		typeName = fullType[idx+1:]
	} else {
		typeName = fullType
	}

	for _, pkg := range pkgs {
		if pkgPath != "" && !strings.HasSuffix(pkg.PkgPath, pkgPath) {
			continue
		}

		scope := pkg.Types.Scope()
		obj := scope.Lookup(typeName)
		if obj == nil {
			continue
		}

		tn, ok := obj.(*types.TypeName)
		if !ok {
			continue
		}

		iface, ok := tn.Type().Underlying().(*types.Interface)
		if !ok {
			continue
		}

		return iface.Complete(), tn, pkg
	}

	return nil, nil, nil
}

func extractDocs(pkg *packages.Package, typeName string) map[string]string {
	docs := map[string]string{}

	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}

			for _, spec := range gen.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok || ts.Name.Name != typeName {
					continue
				}

				it, ok := ts.Type.(*ast.InterfaceType)
				if !ok {
					continue
				}

				for _, field := range it.Methods.List {
					if len(field.Names) == 0 {
						continue
					}
					name := field.Names[0].Name

					if field.Doc != nil {
						docs[name] = field.Doc.Text()
					}
				}
			}
		}
	}

	return docs
}

func typeString(t types.Type) string {
	return types.TypeString(t, func(p *types.Package) string {
		return p.Name()
	})
}
