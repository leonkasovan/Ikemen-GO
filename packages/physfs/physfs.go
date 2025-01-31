package physfs

/*
#include <stdlib.h>
#include "physfs.h"
*/
import "C"
import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unsafe"
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
	if n <= 0 {
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

// io.Reader interface
func (f *File) ReadAt(p []byte, off int64) (n int, err error) {
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return 0, err
	}
	return f.Read(p)
}

func (f *File) Write(p []byte) (n int, err error) {
	n = int(C.PHYSFS_writeBytes((*C.PHYSFS_File)(f), unsafe.Pointer(&p[0]), C.PHYSFS_uint64(len(p))))
	if n < 0 {
		return 0, errors.New("failed to write to file")
	}
	return n, nil
}

func (f *File) WriteAt(p []byte, off int64) (n int, err error) {
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return 0, err
	}
	return f.Write(p)
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
	// fmt.Printf("[physfs] OpenRead(%v) [%v]\n", filename, filepath.Clean(filename))
	cFilename := C.CString(filepath.Clean(filename))
	defer C.free(unsafe.Pointer(cFilename))
	return (*File)(C.PHYSFS_openRead(cFilename))
}

// OpenWrite opens a file for writing.
func OpenWrite(filename string) *File {
	// fmt.Printf("[physfs] OpenWrite(%v) [%v]\n", filename, filepath.Clean(filename))
	cFilename := C.CString(filepath.Clean(filename))
	defer C.free(unsafe.Pointer(cFilename))
	return (*File)(C.PHYSFS_openWrite(cFilename))
}

// OpenAppend opens a file for appending.
func OpenAppend(filename string) *File {
	fmt.Printf("[physfs] OpenAppend(%v) [%v]\n", filename, filepath.Clean(filename))
	cFilename := C.CString(filepath.Clean(filename))
	defer C.free(unsafe.Pointer(cFilename))
	return (*File)(C.PHYSFS_openAppend(cFilename))
}

// Close closes a file.
func Close(f *File) {
	C.PHYSFS_close((*C.PHYSFS_File)(f))
}

// Exists checks if a file/directory exists.
func Exists(filename string) bool {
	cFilename := C.CString(filename)
	defer C.free(unsafe.Pointer(cFilename))
	return C.PHYSFS_exists(cFilename) != 0
}

// Exists checks if a file exists (and not a directory).
func FileExist(filename string) bool {
	var stat C.PHYSFS_Stat
	cFilename := C.CString(filename)
	defer C.free(unsafe.Pointer(cFilename))

	if C.PHYSFS_stat(cFilename, &stat) == 0 {
		return false
	}
	if stat.filetype == C.PHYSFS_FILETYPE_DIRECTORY {
		return false
	}
	return C.PHYSFS_exists(cFilename) != 0
}

// Exists checks if a directory exists (and not a file).
func DirExists(filename string) bool {
	var stat C.PHYSFS_Stat
	cFilename := C.CString(filename)
	defer C.free(unsafe.Pointer(cFilename))

	if C.PHYSFS_stat(cFilename, &stat) == 0 {
		return false
	}
	return stat.filetype == C.PHYSFS_FILETYPE_DIRECTORY
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
	file := OpenRead(filename)
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

// ReadAll reads the content of the file and returns it as a string.
func ReadAll(file *File) ([]byte, error) {
	if file == nil {
		return nil, errors.New("failed to open file")
	}

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
	filename = filepath.Clean(filename)
	fmt.Printf("[physfs.go] Stat(%v)\n", filename)
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

// IsDirectory checks if the given path is a directory.
func IsDirectory(path string) (bool, error) {
	var stat C.PHYSFS_Stat
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	if C.PHYSFS_stat(cPath, &stat) == 0 {
		return false, errors.New("failed to stat path")
	}

	return stat.filetype == C.PHYSFS_FILETYPE_DIRECTORY, nil
}

// GetError returns the last error message.
func GetError() string {
	return C.GoString(C.PHYSFS_getErrorByCode(C.PHYSFS_getLastErrorCode()))
}
