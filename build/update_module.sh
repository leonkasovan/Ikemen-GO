#!/bin/bash
# change glfw with module name to be updated

sed -i '/glfw/d' go.sum
sed -i '/glfw/d' go.mod
rm -r ~/go/pkg/mod/github.com/leonkasovan/glfw
rm -r ~/go/pkg/mod/cache/
go mod tidy