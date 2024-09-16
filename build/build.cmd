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

go build -tags="glfw,gl" -trimpath -v -ldflags "-s -w -H windowsgui" -o ./bin/Ikemen_Go_GLFW.exe ./src
copy bin\Ikemen_Go_GLFW.exe "f:\PortableApps\Mugen Ikemen\Super Crazy Jam Season 1\"

pause