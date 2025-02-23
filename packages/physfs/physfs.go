package physfs

/*
#include <stdlib.h>
#include "physfs.h"
PHYSFS_EnumerateCallbackResult goWalkCallback(void *data, char *origdir, char *fname);
*/
import "C"
import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"
)

type File C.PHYSFS_File

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
	filename = filepath.Clean(filename)
	if runtime.GOOS == "windows" {
		filename = strings.Replace(filename, "\\", "/", -1)
	}
	cFilename := C.CString(filename)
	defer C.free(unsafe.Pointer(cFilename))
	return (*File)(C.PHYSFS_openRead(cFilename))
}

// OpenWrite opens a file for writing.
func OpenWrite(filename string) *File {
	filename = filepath.Clean(filename)
	if runtime.GOOS == "windows" {
		filename = strings.Replace(filename, "\\", "/", -1)
	}
	cFilename := C.CString(filename)
	defer C.free(unsafe.Pointer(cFilename))
	file := (*File)(C.PHYSFS_openWrite(cFilename))
	if file == nil {
		_, err := os.Stat(filepath.Dir(filename))
		if os.IsNotExist(err) {
			if err := os.MkdirAll(filepath.Dir(filename), os.ModePerm); err != nil {
				fmt.Println(err)
				return nil
			}
		}
		file = (*File)(C.PHYSFS_openWrite(cFilename))
	}
	return file
}

// OpenAppend opens a file for appending.
func OpenAppend(filename string) *File {
	filename = filepath.Clean(filename)
	if runtime.GOOS == "windows" {
		filename = strings.Replace(filename, "\\", "/", -1)
	}
	cFilename := C.CString(filename)
	defer C.free(unsafe.Pointer(cFilename))
	return (*File)(C.PHYSFS_openAppend(cFilename))
}

// Close closes a file.
func Close(f *File) {
	C.PHYSFS_close((*C.PHYSFS_File)(f))
}

// Exists checks if a file/directory exists.
func Exists(filename string) bool {
	filename = filepath.Clean(filename)
	if runtime.GOOS == "windows" {
		filename = strings.Replace(filename, "\\", "/", -1)
	}
	cFilename := C.CString(filename)
	defer C.free(unsafe.Pointer(cFilename))
	return C.PHYSFS_exists(cFilename) != 0
}

// Exists checks if a file exists (and not a directory).
func FileExist(filename string) bool {
	var stat C.PHYSFS_Stat
	filename = filepath.Clean(filename)
	if runtime.GOOS == "windows" {
		filename = strings.Replace(filename, "\\", "/", -1)
	}
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
	filename = filepath.Clean(filename)
	if runtime.GOOS == "windows" {
		filename = strings.Replace(filename, "\\", "/", -1)
	}
	cFilename := C.CString(filename)
	defer C.free(unsafe.Pointer(cFilename))

	if C.PHYSFS_stat(cFilename, &stat) == 0 {
		return false
	}
	return stat.filetype == C.PHYSFS_FILETYPE_DIRECTORY
}

// SetWriteDir sets the write directory.
func SetWriteDir(newDir string) bool {
	newDir = filepath.Clean(newDir)
	if runtime.GOOS == "windows" {
		newDir = strings.Replace(newDir, "\\", "/", -1)
	}
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
		return nil, fmt.Errorf("failed to enumerate files in %v", dir)
	}
	defer C.PHYSFS_freeList(unsafe.Pointer(files))

	var fileList []string
	pfile := files
	for {
		if *pfile == nil {
			break
		}
		fileList = append(fileList, C.GoString(*pfile))
		pfile = (**C.char)(unsafe.Pointer(uintptr(unsafe.Pointer(pfile)) + unsafe.Sizeof(*pfile)))
	}
	return fileList, nil
}

/*
	FindFile returns full path of file in the specified directory. filename is incasesentive. return empty string if file not found

Usage:

		validFilePath := physfs.FindFile("data", "system.def")
		validFilePath := physfs.FindFile("data/", "system.def")
		validFilePath := physfs.FindFile("data", "basics/system.def")
	    if validFilePath == "" {
	        fmt.Printf("FAIL")
	    } else {
	        fmt.Printf("Found in %v", validFilePath)
	    }
*/
func FindFile(dir string, filename string) (string, int) {
	// Ensure the directory ends with a '/' if needed
	sep := "/"
	dir = filepath.Clean(dir)
	if len(dir) > 0 && dir[len(dir)-1] != sep[0] {
		dir += sep
	}

	// Sanitize and clean
	filename = strings.Replace(filepath.Clean(filename), "\\", sep, -1)

	// Check if filename consist path separator, then update dir
	if strings.Contains(filename, sep) {
		dir = filepath.Join(dir, filepath.Dir(filename))
		dir += GetDirSeparator()
		filename = filepath.Base(filename)
	}

	if runtime.GOOS == "windows" { // In Windows, we want case sensitive too. So handle it manually.
		cDir := C.CString(dir)
		defer C.free(unsafe.Pointer(cDir))

		files := C.PHYSFS_enumerateFiles(cDir)
		if files == nil {
			fmt.Printf("PHYSFS_enumerateFiles fail in %v\n", dir)
			return "", -2
		}
		defer C.PHYSFS_freeList(unsafe.Pointer(files))

		pfile := files
		for {
			if *pfile == nil {
				break
			}
			if strings.EqualFold(C.GoString(*pfile), filename) {
				if C.GoString(*pfile) == filename {
					return dir + C.GoString(*pfile), 0
				} else {
					return dir + C.GoString(*pfile), 1
				}
			}
			pfile = (**C.char)(unsafe.Pointer(uintptr(unsafe.Pointer(pfile)) + unsafe.Sizeof(*pfile)))
		}
		return "", -1
	} else { // In linux, we can use FileExist to check File case sensitive
		// First, check file existance
		fullpath := dir + filename
		fullpath = filepath.Clean(fullpath)
		if FileExist(fullpath) {
			return fullpath, 0
		}

		// if not found, may be filename is in different case. So enumarate in that directory and compare filename incasesensitive
		cDir := C.CString(dir)
		defer C.free(unsafe.Pointer(cDir))

		files := C.PHYSFS_enumerateFiles(cDir)
		if files == nil {
			fmt.Printf("PHYSFS_enumerateFiles fail in %v\n", dir)
			return "", -2
		}
		defer C.PHYSFS_freeList(unsafe.Pointer(files))

		pfile := files
		for {
			if *pfile == nil {
				break
			}
			if strings.EqualFold(C.GoString(*pfile), filename) {
				return dir + C.GoString(*pfile), 1
			}
			pfile = (**C.char)(unsafe.Pointer(uintptr(unsafe.Pointer(pfile)) + unsafe.Sizeof(*pfile)))
		}
		return "", -1
	}
}

/*
	FindFileExt returns full path of file in directories. filename is incasesentive. return empty string if file not found

Usage:

		validFilePath := physfs.FindFileExt({"data", "font/", "sound"}, "system.def")
		validFilePath := physfs.FindFileExt({"data", "font"}, "basics/system.def")
		validFilePath := physfs.FindFileExt({"data/mrr", "font/abc/"}, "basics/system.def")
	    if validFilePath == "" {
	        fmt.Printf("FAIL")
	    } else {
	        fmt.Printf("Found in %v", validFilePath)
	    }
*/
func FindFileExt(dirs []string, filename string) (string, int) {
	for _, dir := range dirs {
		validPath, rc := FindFile(dir, filename)
		if validPath != "" {
			return validPath, rc
		}
	}
	return "", -1
}

/*
	FindFileMatch returns full path of files that match with pattern in the specified directory. return empty string if file not found

Usage:

		validFilesPath := physfs.FindFileMatch("data", "*.def")
		if len(validFilesPath) == 0 {
			fmt.Printf("FAIL")
	    } else {
			for _, v := range validFilesPath {
				fmt.Printf("Found in %v\n", v)
			}
		}
*/
func FindFileMatch(dir string, filename_pattern string) []string {
	// Ensure the directory ends with a '/' if needed
	sep := GetDirSeparator()
	dir = filepath.Clean(dir)
	if len(dir) > 0 && dir[len(dir)-1] != sep[0] {
		dir += sep
	}

	cDir := C.CString(dir)
	defer C.free(unsafe.Pointer(cDir))
	fileMatchedList := []string{}
	files := C.PHYSFS_enumerateFiles(cDir)
	if files == nil {
		fmt.Printf("Error: PHYSFS_enumerateFiles fail in %v\n", dir)
		return fileMatchedList
	}
	defer C.PHYSFS_freeList(unsafe.Pointer(files))

	pfile := files
	for {
		if *pfile == nil {
			break
		}
		filename := C.GoString(*pfile)
		matched, err := filepath.Match(filename_pattern, filename)
		if err != nil {
			fmt.Printf("Error filepath.Match: %s\n", err)
			return fileMatchedList
		}
		if matched {
			fileMatchedList = append(fileMatchedList, dir+filename)
		}
		pfile = (**C.char)(unsafe.Pointer(uintptr(unsafe.Pointer(pfile)) + unsafe.Sizeof(*pfile)))
	}
	return fileMatchedList
}

// CheckFile will check incase senstive and returns full path of file if exists. return empty string if file not found
/*
Usage:
	validFilePath := physfs.CheckFile("data/system.def")
	validFilePath := physfs.CheckFile("data/other/../System.def")
	validFilePath := physfs.CheckFile("data/basics/Basic_Moves.st")
    if validFilePath == "" {
        fmt.Printf("FAIL")
    } else {
        fmt.Printf("Found in %v", validFilePath)
    }
*/
func CheckFile(fullpath string) (string, int) {
	// fullpath = filepath.Clean(fullpath)

	// First, check file existance
	if FileExist(fullpath) {
		return fullpath, 0
	}

	// if not found, may be filename is in different case. So enumarate in that directory and compare filename incasesensitive via FindFile
	return FindFile(filepath.Dir(fullpath), filepath.Base(fullpath))
}

// GetDirSeparator returns the directory separator.
func GetDirSeparator() string {
	return C.GoString(C.PHYSFS_getDirSeparator())
}

// ReadFile reads the content of the file and returns it as a byte slice.
func ReadFile(filename string) ([]byte, error) {
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
	var stat C.PHYSFS_Stat
	filename = filepath.Clean(filename)
	if runtime.GOOS == "windows" {
		filename = strings.Replace(filename, "\\", "/", -1)
	}
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
		return false, fmt.Errorf("failed to stat %v", path)
	}
	return stat.filetype == C.PHYSFS_FILETYPE_DIRECTORY, nil
}

// GetError returns the last error message.
func GetError() string {
	return C.GoString(C.PHYSFS_getErrorByCode(C.PHYSFS_getLastErrorCode()))
}

// PHYSFS_getBaseDir returns the base directory.
func GetBaseDir() string {
	return C.GoString(C.PHYSFS_getBaseDir())
}

// WalkFunc is the function called for each file and directory found.
type WalkFunc func(path string, isDir bool) error

// Walk walks the directory tree rooted at root, calling walkFn for each file or directory.
func Walk(root string, walkFn WalkFunc) {
	cRoot := C.CString(root)
	defer C.free(unsafe.Pointer(cRoot))
	C.PHYSFS_enumerate(cRoot, (*[0]byte)(unsafe.Pointer(C.goWalkCallback)), unsafe.Pointer(&walkFn))
}

//export goWalkCallback
func goWalkCallback(data unsafe.Pointer, origdir *C.char, fname *C.char) C.PHYSFS_EnumerateCallbackResult {
	// Get full path from origdir and fname
	fullPath := filepath.Join(C.GoString(origdir), C.GoString(fname))

	// Check if it's a directory
	isDir, err := IsDirectory(fullPath)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return C.PHYSFS_ENUM_STOP
	}

	// Call the user-provided callback function
	walkFn := *(*WalkFunc)(data)
	if err := walkFn(fullPath, isDir); err != nil {
		return C.PHYSFS_ENUM_STOP // Stop traversal if error occurs
	}

	// If it's a directory, recurse into it
	if isDir {
		cSubDir := C.CString(fullPath)
		defer C.free(unsafe.Pointer(cSubDir))
		C.PHYSFS_enumerate(cSubDir, (*[0]byte)(unsafe.Pointer(C.goWalkCallback)), data)
	}

	return C.PHYSFS_ENUM_OK // Continue traversal
}
