package physfs

/*
#include <stdlib.h>
#include "physfs.h"
*/
import "C"
import (
    "errors"
    "io"
    "fmt"
    "unsafe"
    "path/filepath"
    "strings"
)

type File C.PHYSFS_File

func normalizePath(path string) string {
    // Clean the path to remove any redundant elements
    cleanPath := filepath.Clean(path)
    fmt.Printf("path=%v cleanPath=%v\n", path, cleanPath)

    // Split the path into components
    components := strings.Split(cleanPath, string(filepath.Separator))

    // Remove leading ".." or "." components
    var normalizedComponents []string
    for _, component := range components {
        if component != ".." && component != "." {
            normalizedComponents = append(normalizedComponents, component)
        }
    }

    // Join the components back into a single path
    normalizedPath := filepath.Join(normalizedComponents...)

    return normalizedPath
}

func (f *File) Read(p []byte) (n int, err error) {
    n = int(C.PHYSFS_readBytes((*C.PHYSFS_File)(f), unsafe.Pointer(&p[0]), C.PHYSFS_uint64(len(p))))
    if n < 0 {
        return 0, io.EOF
    }
    return n, nil
}

func (f *File) Seek(offset int64, whence int) (int64, error) {
    var result C.int
    switch whence {
    case io.SeekStart:
        result = C.PHYSFS_seek((*C.PHYSFS_File)(f), C.PHYSFS_uint64(offset))
    case io.SeekCurrent:
        currentPos := C.PHYSFS_tell((*C.PHYSFS_File)(f))
        result = C.PHYSFS_seek((*C.PHYSFS_File)(f), C.PHYSFS_uint64(currentPos)+C.PHYSFS_uint64(offset))
    case io.SeekEnd:
        fileLength := C.PHYSFS_fileLength((*C.PHYSFS_File)(f))
        result = C.PHYSFS_seek((*C.PHYSFS_File)(f), C.PHYSFS_uint64(fileLength)+C.PHYSFS_uint64(offset))
    default:
        return 0, errors.New("invalid argument")
    }
    if result == 0 {
        return 0, io.EOF
    }
    return int64(C.PHYSFS_tell((*C.PHYSFS_File)(f))), nil
}

func (f *File) Close() error {
    if C.PHYSFS_close((*C.PHYSFS_File)(f)) == 0 {
        return errors.New("failed to close file")
    }
    return nil
}

// Init initializes the PhysicsFS library.
func Init(argv0 string) bool {
    cArgv0 := C.CString(argv0)
    defer C.free(unsafe.Pointer(cArgv0))
    return C.PHYSFS_init(cArgv0) != 0
}

// Deinit deinitializes the PhysicsFS library.
func Deinit() {
    C.PHYSFS_deinit()
}

// Mount mounts an archive.
func Mount(archive, mountPoint string, appendToPath int) bool {
    cArchive := C.CString(archive)
    defer C.free(unsafe.Pointer(cArchive))
    cMountPoint := C.CString(mountPoint)
    defer C.free(unsafe.Pointer(cMountPoint))
    return C.PHYSFS_mount(cArchive, cMountPoint, C.int(appendToPath)) != 0
}

// Unmount unmounts an archive.
func Unmount(archive string) bool {
    cArchive := C.CString(archive)
    defer C.free(unsafe.Pointer(cArchive))
    return C.PHYSFS_unmount(cArchive) != 0
}

// OpenRead opens a file for reading.
func OpenRead(filename string) *File {
    // fmt.Printf("filename=%v [%v]\n", filename, filepath.Clean(filename))
    cFilename := C.CString(filepath.Clean(filename))
    defer C.free(unsafe.Pointer(cFilename))
    return (*File)(C.PHYSFS_openRead(cFilename))
}

// Close closes a file.
func Close(f *File) {
    C.PHYSFS_close((*C.PHYSFS_File)(f))
}

// Exists checks if a file exists.
func Exists(filename string) bool {
    cFilename := C.CString(filename)
    defer C.free(unsafe.Pointer(cFilename))
    return C.PHYSFS_exists(cFilename) != 0
}

// OpenWrite opens a file for writing.
func OpenWrite(filename string) *File {
    cFilename := C.CString(filename)
    defer C.free(unsafe.Pointer(cFilename))
    return (*File)(C.PHYSFS_openWrite(cFilename))
}

// SetWriteDir sets the write directory.
func SetWriteDir(newDir string) bool {
    cNewDir := C.CString(newDir)
    defer C.free(unsafe.Pointer(cNewDir))
    return C.PHYSFS_setWriteDir(cNewDir) != 0
}

// GetSearchPath returns the search path.
func GetSearchPath() ([]string, error) {
    var searchPath **C.char = C.PHYSFS_getSearchPath()
    if searchPath == nil {
        return nil, errors.New("failed to get search path")
    }
    defer C.PHYSFS_freeList(unsafe.Pointer(searchPath))

    var paths []string
    for {
        if *searchPath == nil {
            break
        }
        paths = append(paths, C.GoString(*searchPath))
        searchPath = (**C.char)(unsafe.Pointer(uintptr(unsafe.Pointer(searchPath)) + unsafe.Sizeof(*searchPath)))
    }
    return paths, nil
}

// EnumerateFiles returns a list of files in the specified directory.
func EnumerateFiles(dir string) ([]string, error) {
    cDir := C.CString(dir)
    defer C.free(unsafe.Pointer(cDir))

    files := C.PHYSFS_enumerateFiles(cDir)
    if files == nil {
        return nil, errors.New("failed to enumerate files")
    }
    defer C.PHYSFS_freeList(unsafe.Pointer(files))

    var fileList []string
    for {
        if *files == nil {
            break
        }
        fileList = append(fileList, C.GoString(*files))
        files = (**C.char)(unsafe.Pointer(uintptr(unsafe.Pointer(files)) + unsafe.Sizeof(*files)))
    }
    return fileList, nil
}

// GetDirSeparator returns the directory separator.
func GetDirSeparator() string {
    return C.GoString(C.PHYSFS_getDirSeparator())
}

// ReadFile reads the content of the file and returns it as a byte slice.
func ReadFile(filename string) ([]byte, error) {
    // fmt.Printf("physfs.ReadFile(%v)\n", filepath.Clean(filename))
    file := OpenRead(filepath.Clean(filename))
    if file == nil {
        return nil, errors.New("failed to open file")
    }
    defer file.Close()

    fileLength := C.PHYSFS_fileLength((*C.PHYSFS_File)(file))
    if fileLength < 0 {
        return nil, errors.New("failed to get file length")
    }

    if fileLength == 0 {
        return nil, nil
    }

    buffer := make([]byte, fileLength)
    n, err := file.Read(buffer)
    if err != nil && err != io.EOF {
        return nil, err
    }
    return buffer[:n], nil
}

// FileInfo represents information about a file.
type FileInfo struct {
    Name     string
    Exists   bool
    IsDir    bool
    ModTime  int64
    FileSize int64
}

// Stat retrieves information about a file.
func Stat(filename string) (*FileInfo, error) {
    var stat C.PHYSFS_Stat
    cFilename := C.CString(filename)
    defer C.free(unsafe.Pointer(cFilename))

    if C.PHYSFS_stat(cFilename, &stat) == 0 {
        return nil, errors.New("failed to stat file")
    }

    fileInfo := &FileInfo{
        Name:     filename,
        Exists:   stat.filetype != C.PHYSFS_FILETYPE_OTHER,
        IsDir:    stat.filetype == C.PHYSFS_FILETYPE_DIRECTORY,
        ModTime:  int64(stat.modtime),
        FileSize: int64(stat.filesize),
    }

    return fileInfo, nil
}