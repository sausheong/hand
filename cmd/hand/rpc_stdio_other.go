//go:build !darwin && !linux

package main

var prepareStdioRPC = unsupportedStdioRPC
