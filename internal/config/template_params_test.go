package config

import (
	"reflect"
	"testing"
	"text/template"
)

func TestRequiredTemplateParams(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		want    []string
		wantErr bool
	}{
		{name: "direct", source: `{{.Params.target}}`, want: []string{"target"}},
		{name: "whitespace", source: `{{ .Params.target }}`, want: []string{"target"}},
		{name: "function argument", source: `{{printf "%s" .Params.target}}`, want: []string{"target"}},
		{name: "multiple sorted", source: `{{.Params.z}} {{.Params.a}} {{.Params.z}}`, want: []string{"a", "z"}},
		{name: "dynamic index", source: `{{index .Params "target"}}`, wantErr: true},
		{name: "dynamic root index", source: `{{index . "Params"}}`, wantErr: true},
		{name: "whole params map", source: `{{.Params}}`, wantErr: true},
		{name: "indirect variable", source: `{{$root := .}}{{$root.Params.target}}`, wantErr: true},
		{name: "render root", source: `{{.}}`, wantErr: true},
		{name: "function receives root", source: `{{printf "%v" .}}`, wantErr: true},
		{name: "alias root", source: `{{$root := .}}{{$root}}`, wantErr: true},
		{
			name:   "associated template receives audited root",
			source: `{{define "child"}}{{.Params.target}}{{end}}{{template "child" .}}`,
			want:   []string{"target"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := template.New("test").Parse(tt.source)
			if err != nil {
				t.Fatalf("parse template: %v", err)
			}
			got, err := RequiredTemplateParams(tmpl)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected template to be rejected, got params %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("RequiredTemplateParams failed: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("required params = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateSensitiveParams(t *testing.T) {
	tests := []struct {
		name    string
		params  []string
		wantErr bool
	}{
		{name: "empty"},
		{name: "valid", params: []string{"token", "Password_2"}},
		{name: "invalid name", params: []string{"bad-key"}, wantErr: true},
		{name: "case collision", params: []string{"token", "TOKEN"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSensitiveParams(tt.params)
			if tt.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}
