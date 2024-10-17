@echo off
cd ..
set CGO_ENABLED = 1
set GOOS = windows

if not exist go.mod (
	echo Missing dependencies, please run get.cmd
	echo.
	pause
	exit
)
if not exist bin (
	MKDIR bin
) 

echo Building Ikemen GO...
echo. 

rem go build -tags="gl,glfw" -trimpath -v -ldflags "-s -w -H=windowsgui" -o ./bin/Ikemen_GO_Batch.exe ./src
rem go build -tags="gl,sdl" -trimpath -v -ldflags "-s -w -H=windowsgui" -o ./bin/Ikemen_GO_Batch.exe ./src
go build -tags="gles,sdl" -trimpath -v -ldflags "-s -w -H=windowsgui" -o ./bin/Ikemen_GO_Batch.exe ./src
