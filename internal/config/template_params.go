package config

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"text/template"
	"text/template/parse"
)

var templateParamNameRegex = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)

// RequiredTemplateParams extracts direct .Params.name references from a parsed
// template. Dynamic access is rejected because it cannot be validated before
// hooks execute.
func RequiredTemplateParams(tmpl *template.Template) ([]string, error) {
	if tmpl == nil {
		return nil, fmt.Errorf("template no compilado")
	}

	required := make(map[string]struct{})
	for _, associated := range tmpl.Templates() {
		if associated.Tree == nil || associated.Tree.Root == nil {
			continue
		}
		if err := walkTemplateNode(associated.Tree.Root, required); err != nil {
			return nil, err
		}
	}

	params := make([]string, 0, len(required))
	for name := range required {
		params = append(params, name)
	}
	sort.Strings(params)
	return params, nil
}

func validateSensitiveParams(params []string) error {
	seen := make(map[string]string, len(params))
	for _, name := range params {
		if !templateParamNameRegex.MatchString(name) {
			return fmt.Errorf("nombre inválido en sensitive_params: %q", name)
		}
		normalized := strings.ToUpper(name)
		if existing, found := seen[normalized]; found {
			return fmt.Errorf(
				"sensitive_params contiene nombres equivalentes: %q y %q",
				existing,
				name,
			)
		}
		seen[normalized] = name
	}
	return nil
}

func walkTemplateNode(node parse.Node, required map[string]struct{}) error {
	if node == nil {
		return nil
	}

	switch typed := node.(type) {
	case *parse.ListNode:
		for _, child := range typed.Nodes {
			if err := walkTemplateNode(child, required); err != nil {
				return err
			}
		}
	case *parse.ActionNode:
		return walkTemplateNode(typed.Pipe, required)
	case *parse.PipeNode:
		for _, command := range typed.Cmds {
			if err := walkTemplateNode(command, required); err != nil {
				return err
			}
		}
	case *parse.CommandNode:
		for _, argument := range typed.Args {
			if identifier, ok := argument.(*parse.IdentifierNode); ok && identifier.Ident == "index" {
				return fmt.Errorf("acceso dinámico con 'index' no permitido en templates de acciones")
			}
			if err := walkTemplateNode(argument, required); err != nil {
				return err
			}
		}
	case *parse.FieldNode:
		if len(typed.Ident) == 0 || typed.Ident[0] != "Params" {
			return nil
		}
		if len(typed.Ident) != 2 || !templateParamNameRegex.MatchString(typed.Ident[1]) {
			return fmt.Errorf("acceso dinámico o inválido a Params no permitido: %s", typed.String())
		}
		required[typed.Ident[1]] = struct{}{}
	case *parse.VariableNode:
		for _, identifier := range typed.Ident {
			if identifier == "Params" {
				return fmt.Errorf("acceso indirecto a Params no permitido: %s", typed.String())
			}
		}
	case *parse.ChainNode:
		if err := walkTemplateNode(typed.Node, required); err != nil {
			return err
		}
	case *parse.DotNode:
		return fmt.Errorf("renderizar o aliasar el contexto raíz no está permitido")
	case *parse.IfNode:
		return walkTemplateBranch(&typed.BranchNode, required)
	case *parse.RangeNode:
		return walkTemplateBranch(&typed.BranchNode, required)
	case *parse.WithNode:
		return walkTemplateBranch(&typed.BranchNode, required)
	case *parse.TemplateNode:
		return walkTemplateInvocationPipe(typed.Pipe, required)
	}
	return nil
}

func walkTemplateInvocationPipe(pipe *parse.PipeNode, required map[string]struct{}) error {
	if pipe == nil {
		return nil
	}
	if len(pipe.Cmds) == 1 && len(pipe.Cmds[0].Args) == 1 {
		if _, ok := pipe.Cmds[0].Args[0].(*parse.DotNode); ok {
			return nil
		}
	}
	return walkTemplateNode(pipe, required)
}

func walkTemplateBranch(branch *parse.BranchNode, required map[string]struct{}) error {
	if err := walkTemplateNode(branch.Pipe, required); err != nil {
		return err
	}
	if err := walkTemplateNode(branch.List, required); err != nil {
		return err
	}
	return walkTemplateNode(branch.ElseList, required)
}
