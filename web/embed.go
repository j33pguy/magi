package web

import "embed"

//go:embed templates/* static/*
var WebFS embed.FS
