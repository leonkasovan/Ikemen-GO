cls
go build -tags="glfw,gl" -trimpath -v -ldflags "-s -w -H windowsgui" -o ./bin/Ikemen_Go_GLFW_Batch.exe ./src
cd bin
Ikemen_Go_GLFW_Batch.exe -p1 kfmZ -p2 kfmZ -s WindMill