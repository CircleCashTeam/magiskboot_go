# Magiskboot golang impl
This is magiskboot reimpl by golang

# Function avaliable
|Function|Avaliable|
|---------|--------|
|Cleanup  | ✅    |
|Sha1     | ✅    |
|Split    | ✅    |
|Unpack   | ❎    |
|Repack   | ❎    |
|Verify   | ❎    |
|Sign     | ❎    |
|Decompress| ✅    |
|Compress | ✅    |
|Hexpatch | ✅    |
|Cpio     | ✅    |
|Dtb      | ❎    |
|Extract  | ✅    |
# Build
## go
```bash
go build -o magiskboot cmd/main.go
```
## tinygo
```bash
tinygo build -o magiskboot cmd/main.go
```
### Small size
```bash
tinygo build -opt=z -no-debug -size short -o magiskboot cmd/main.go
```
## build for windows
```bash
# on linux
GOOS="windows" GOARCH="amd64" go build -o magiskboot.exe cmd/main.go
```