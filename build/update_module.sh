#!/bin/bash
# Script to update leonkasovan's module from github

sed -i '/leonkasovan/d' go.sum
sed -i '/leonkasovan/d' go.mod
rm -r ~/go/pkg/mod/github.com/leonkasovan
rm -r ~/go/pkg/mod/cache
go mod tidy