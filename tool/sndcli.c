#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>
#include <string.h>
#include <sndfile.h>
#include <dirent.h>
#include <sys/stat.h>
#include <libgen.h>

// ---- SND headers (Preserved from original) ----
#pragma pack(push, 1)
typedef struct {
    char signature[12];   // "ElecbyteSnd"
    uint16_t nHiVer;
    uint16_t nLoVer;
    uint32_t cSounds;
    uint32_t oFirst;
    char comment[488];
} Header;

typedef struct {
    uint32_t oNext;
    uint32_t cbSubfile;
    uint32_t nGroup;
    uint32_t nIndex;
} SubfileHeader;
#pragma pack(pop)

// ---- Memory-backed file for libsndfile (Preserved from original) ----
typedef struct {
    const unsigned char* data;
    sf_count_t size;
    sf_count_t pos;
} MemFile;

static sf_count_t mem_get_filelen(void* user_data) {
    return ((MemFile*)user_data)->size;
}
static sf_count_t mem_seek(sf_count_t offset, int whence, void* user_data) {
    MemFile* m = (MemFile*)user_data;
    sf_count_t np = m->pos;
    if (whence == SEEK_SET) np = offset;
    else if (whence == SEEK_CUR) np += offset;
    else if (whence == SEEK_END) np = m->size + offset;
    if (np < 0 || np > m->size) return -1;
    m->pos = np;
    return m->pos;
}
static sf_count_t mem_read(void* ptr, sf_count_t count, void* user_data) {
    MemFile* m = (MemFile*)user_data;
    if (m->pos + count > m->size) count = m->size - m->pos;
    memcpy(ptr, m->data + m->pos, count);
    m->pos += count;
    return count;
}
static sf_count_t mem_write(const void* ptr, sf_count_t count, void* user_data) {
    (void)ptr; (void)user_data; (void)count;
    return 0; // read-only input
}
static sf_count_t mem_tell(void* user_data) {
    return ((MemFile*)user_data)->pos;
}

// =============================================================
//   CORE CONVERSION LOGIC
// =============================================================

/**
 * Generic helper to convert in-memory WAV data to a target format.
 * * @param wavData Raw input bytes
 * @param wavSize Size of input bytes
 * @param outSize Pointer to store the size of the resulting buffer
 * @param targetFormat libsndfile SF_FORMAT_* flags (Container | Codec)
 * @param ext File extension for the temporary file (e.g., "mp3", "flac")
 */
static unsigned char* convert_generic(const unsigned char* wavData, size_t wavSize,
                                      size_t* outSize, int targetFormat, const char* ext) {
    // 1. Setup Input Virtual I/O
    SF_VIRTUAL_IO vio = {
        mem_get_filelen, mem_seek, mem_read, mem_write, mem_tell
    };
    MemFile mem = { wavData, (sf_count_t)wavSize, 0 };

    SF_INFO sfinfo = {0};
    SNDFILE* snd = sf_open_virtual(&vio, SFM_READ, &sfinfo, &mem);
    if (!snd) {
        fprintf(stderr, "libsndfile: failed to decode input WAV\n");
        return NULL;
    }

    // 2. Format Specific Checks (e.g., GSM Requirements)
    int subType = targetFormat & SF_FORMAT_SUBMASK;
    if (subType == SF_FORMAT_GSM610) {
        if (sfinfo.samplerate != 8000 || sfinfo.channels != 1) {
            fprintf(stderr, "Skip: GSM 6.10 requires 8000Hz Mono input (Current: %dHz %dch)\n",
                    sfinfo.samplerate, sfinfo.channels);
            sf_close(snd);
            return NULL;
        }
    }

    // 3. Prepare Temporary File Output
    char tmpName[64];
    snprintf(tmpName, sizeof(tmpName), "tmp_convert.%s", ext);

    SF_INFO outInfo = sfinfo;
    outInfo.format = targetFormat;

    SNDFILE* out = sf_open(tmpName, SFM_WRITE, &outInfo);
    if (!out) {
        // Specific check for missing MP3 support in library
        if ((targetFormat & SF_FORMAT_SUBMASK) == SF_FORMAT_MPEG_LAYER_III) {
             int err = sf_error(NULL);
             if (err == SF_ERR_UNRECOGNISED_FORMAT) {
                 fprintf(stderr, "Error: Your libsndfile does not support MP3 (need v1.1.0+ with LAME).\n");
             }
        }
        fprintf(stderr, "libsndfile: cannot open encoder for %s: %s\n", ext, sf_strerror(NULL));
        sf_close(snd);
        return NULL;
    }

    // 4. Copy Loop
    short pcm[8192 * 2];
    sf_count_t read_count;
    while ((read_count = sf_readf_short(snd, pcm, 8192)) > 0) {
        sf_writef_short(out, pcm, read_count);
    }

    sf_close(out);
    sf_close(snd);

    // 5. Read file back into memory
    FILE* tmp = fopen(tmpName, "rb");
    if (!tmp) return NULL;

    fseek(tmp, 0, SEEK_END);
    long size = ftell(tmp);
    fseek(tmp, 0, SEEK_SET);

    unsigned char* buf = malloc(size);
    if (buf) {
        fread(buf, 1, size, tmp);
    }

    fclose(tmp);
    remove(tmpName); // Delete temp file

    *outSize = size;
    return buf;
}

// ---- Wrappers ----

unsigned char* wav_to_ogg(const unsigned char* data, size_t len, size_t* outLen) {
    return convert_generic(data, len, outLen, SF_FORMAT_OGG | SF_FORMAT_VORBIS, "ogg");
}

void print_all_supported_formats(void) {
    int count;
    // 1. Get the number of supported major formats
    sf_command(NULL, SFC_GET_FORMAT_MAJOR_COUNT, &count, sizeof(int));

    printf("Supported Major Formats:\n");
    for (int i = 0; i < count; i++) {
        SF_FORMAT_INFO info;
        info.format = i;
        // 2. Iterate through them to get names and extensions
        sf_command(NULL, SFC_GET_FORMAT_MAJOR, &info, sizeof(info));
        printf("  0x%08x : %s (extension: .%s)\n", info.format, info.name, info.extension);
    }
    printf("\n\n");

    // 3. Get the number of supported sub formats
    sf_command(NULL, SFC_GET_FORMAT_SUBTYPE_COUNT, &count, sizeof(int));

    printf("Supported Sub Formats:\n");
    for (int i = 0; i < count; i++) {
        SF_FORMAT_INFO info;
        info.format = i;
        // 4. Iterate through them to get names and extensions
        sf_command(NULL, SFC_GET_FORMAT_SUBTYPE, &info, sizeof(info));
        printf("  0x%08x : %s (extension: .%s)\n", info.format, info.name, info.extension);
    }
    printf("\n\n");
}

// =============================================================
//   MAIN REBUILDER
// =============================================================

typedef struct {
    const char* workdir;
    double threshold;
    int recursive;
    int verbose;
} Options;

// Process a single SND file
int process_snd_file(const char* input_path, Options opts) {
    // Generate output filename: z.<basename> in the same directory as input
    char output_path[512];
    char* full_path = strdup(input_path);
    
    // Find the last directory separator
    char* last_sep = strrchr(full_path, '/');
    if (!last_sep) last_sep = strrchr(full_path, '\\');
    
    char* basename;
    char dir_path[512];
    
    if (!last_sep) {
        // File in current directory
        basename = full_path;
        strcpy(dir_path, ".");
    } else {
        // Extract directory and basename
        basename = last_sep + 1;
        *last_sep = '\0';
        strcpy(dir_path, full_path);
    }
    
    // skip if basename already starts with 'z.'
    if (strncmp(basename, "z.", 2) == 0) {
        free(full_path);
        return 0;
    }
    
    // Output goes to the same directory as input
    snprintf(output_path, sizeof(output_path), "%s/z.%s", dir_path, basename);
    free(full_path);

    FILE* in = fopen(input_path, "rb");
    FILE* out = fopen(output_path, "wb");
    if (!in || !out) {
        perror("File error");
        if (in) fclose(in);
        if (out) fclose(out);
        return 0;
    }

    Header hdr;
    fread(&hdr, sizeof(hdr), 1, in);
    if (memcmp(hdr.signature, "ElecbyteSnd", 11) != 0) {
        fprintf(stderr, "Not a valid M.U.G.E.N SND file: %s\n", input_path);
        fclose(in);
        fclose(out);
        return 0;
    }
    fwrite(&hdr, sizeof(hdr), 1, out);

    // if (opts.verbose) {
        printf("Processing: %s -> %s\n", input_path, output_path);
    // }

    fseek(in, hdr.oFirst, SEEK_SET);
    uint32_t nextOffset = hdr.oFirst;
    int count_converted = 0, count_skipped = 0;

    for (uint32_t i = 0; i < hdr.cSounds; i++) {
        fseek(in, nextOffset, SEEK_SET);
        SubfileHeader sub;
        fread(&sub, sizeof(sub), 1, in);

        unsigned char* wavData = malloc(sub.cbSubfile);
        if (!wavData) {
            fprintf(stderr, "Memory allocation error\n");
            break;
        }

        fread(wavData, sub.cbSubfile, 1, in);

        size_t newSize = 0;
        unsigned char* newData = NULL;

        // Convert to OGG
        newData = wav_to_ogg(wavData, sub.cbSubfile, &newSize);

        long curPos = ftell(out);
        SubfileHeader newSub;
        newSub.nGroup = sub.nGroup;
        newSub.nIndex = sub.nIndex;
        
        // Compression ratio threshold check
        int useConverted = 0;
        double ratio = 1.0;
        if (newData && newSize > 0) {
            ratio = (double)newSize / (double)sub.cbSubfile;
            if (ratio <= opts.threshold) {
                useConverted = 1;
                count_converted++;
            } else {
                count_skipped++;
            }
        } else {
            count_skipped++;
        }
        
        newSub.cbSubfile = useConverted ? newSize : sub.cbSubfile;
        newSub.oNext = curPos + sizeof(SubfileHeader) + newSub.cbSubfile;

        fwrite(&newSub, sizeof(newSub), 1, out);
        fwrite(useConverted ? newData : wavData, newSub.cbSubfile, 1, out);
        
        if (opts.verbose) {
            printf("  [%u,%u]: %lu -> %lu bytes (%.1f%%)%s\n",
               sub.nGroup, sub.nIndex,
               (unsigned long)sub.cbSubfile, (unsigned long)newSize, ratio * 100,
               useConverted ? " [converted]" : " [skipped]");
        }

        free(wavData);
        if (newData) free(newData);

        nextOffset = sub.oNext;
    }

    // if (opts.verbose) {
        // Get file size ratio
        fseek(in, 0, SEEK_END);
        long inSize = ftell(in);
        fseek(out, 0, SEEK_END);
        long outSize = ftell(out);
        double totalRatio = (double)outSize / (double)inSize * 100.0;
        // print original size vs new size
        printf("\t-> Original Size: %lu bytes, Converted Size: %lu bytes \t-> Overall Ratio: %.1f%%\n",
               (unsigned long)inSize, (unsigned long)outSize, totalRatio);
        printf("\t-> Converted: %d, Skipped: %d\n\n", count_converted, count_skipped);
    // }

    fclose(in);
    fclose(out);

    return 1;
}

// Recursively scan directory for *.snd files
void scan_directory(const char* path, Options opts) {
    DIR* dir = opendir(path);
    if (!dir) {
        perror("opendir");
        return;
    }

    struct dirent* entry;
    while ((entry = readdir(dir)) != NULL) {
        if (entry->d_name[0] == '.') continue;

        char full_path[512];
        snprintf(full_path, sizeof(full_path), "%s/%s", path, entry->d_name);

        struct stat st;
        if (stat(full_path, &st) == -1) continue;

        if (S_ISDIR(st.st_mode)) {
            if (opts.recursive) {
                scan_directory(full_path, opts);
            }
        } else if (S_ISREG(st.st_mode)) {
            // Check if file ends with .snd
            int len = strlen(entry->d_name);
            if (len > 4 && strcmp(entry->d_name + len - 4, ".snd") == 0) {
                process_snd_file(full_path, opts);
            }
        }
    }

    closedir(dir);
}

int main(int argc, char** argv) {
    if (argc < 2) {
        printf("Usage: %s <workdir> [options]\n", argv[0]);
        printf("Options:\n");
        printf("  -t <ratio>   Compression ratio threshold (default: 0.30 = 30%%)\n");
        printf("  -r           Recursive directory scan\n");
        printf("  -i           Verbose information\n");
        printf("\nExample: %s ./data -t 0.50 -r -i\n", argv[0]);
        return 1;
    }

    Options opts = {
        .workdir = argv[1],
        .threshold = 0.30,
        .recursive = 0,
        .verbose = 0
    };

    // Parse command-line options
    for (int i = 2; i < argc; i++) {
        if (strcmp(argv[i], "-r") == 0) {
            opts.recursive = 1;
        } else if (strcmp(argv[i], "-i") == 0) {
            opts.verbose = 1;
        } else if (strcmp(argv[i], "-t") == 0 && i + 1 < argc) {
            opts.threshold = atof(argv[++i]);
        }
    }

    // Verify working directory exists
    struct stat st;
    if (stat(opts.workdir, &st) == -1 || !S_ISDIR(st.st_mode)) {
        fprintf(stderr, "Error: Invalid working directory: %s\n", opts.workdir);
        return 1;
    }

    if (opts.verbose) {
        printf("=== SND Converter ===\n");
        printf("Working directory: %s\n", opts.workdir);
        printf("Compression threshold: %.0f%%\n", opts.threshold * 100);
        printf("Recursive: %s\n", opts.recursive ? "Yes" : "No");
        printf("====================\n\n");
    }

    scan_directory(opts.workdir, opts);

    // if (opts.verbose) {
        printf("Done!\n");
    // }

    return 0;
}
