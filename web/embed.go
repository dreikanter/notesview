package web

import "embed"

//go:embed static/*
var StaticFS embed.FS

//go:embed templates/*.html
var TemplatesFS embed.FS
