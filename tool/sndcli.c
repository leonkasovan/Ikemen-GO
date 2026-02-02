#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>
#include <string.h>
#include <sndfile.h>

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

int main(int argc, char** argv) {

	print_all_supported_formats();

    if (argc < 4) {
        printf("Usage: %s <mode> input.snd output.snd\n", argv[0]);
        printf("Modes:\n");
        printf("  --ogg    (Vorbis)\n");
        return 1;
    }

    enum { MODE_MP3, MODE_OGG, MODE_FLAC, MODE_ADPCM } mode;
    const char* modeName = "";

    if (strcmp(argv[1], "--ogg") == 0)   { mode = MODE_OGG; modeName = "OGG"; }
    else {
        fprintf(stderr, "Unknown mode: %s\n", argv[1]);
        return 1;
    }

    FILE* in = fopen(argv[2], "rb");
    FILE* out = fopen(argv[3], "wb");
    if (!in || !out) {
        perror("File error");
        return 1;
    }

    Header hdr;
    fread(&hdr, sizeof(hdr), 1, in);
    if (memcmp(hdr.signature, "ElecbyteSnd", 11) != 0) {
        fprintf(stderr, "Not a valid M.U.G.E.N SND file\n");
        fclose(in); fclose(out);
        return 1;
    }
    fwrite(&hdr, sizeof(hdr), 1, out);

    fseek(in, hdr.oFirst, SEEK_SET);
    uint32_t nextOffset = hdr.oFirst;

    for (uint32_t i = 0; i < hdr.cSounds; i++) {
        fseek(in, nextOffset, SEEK_SET);
        SubfileHeader sub;
        fread(&sub, sizeof(sub), 1, in);

        unsigned char* wavData = malloc(sub.cbSubfile);
        if (!wavData) { fprintf(stderr, "Memory allocation error\n"); break; }

        fread(wavData, sub.cbSubfile, 1, in);

        size_t newSize = 0;
        unsigned char* newData = NULL;

        // Route to appropriate wrapper
        switch (mode) {
            case MODE_OGG:    newData = wav_to_ogg(wavData, sub.cbSubfile, &newSize); break;
            default: break;
        }

        long curPos = ftell(out);
        SubfileHeader newSub;
        newSub.nGroup = sub.nGroup;
        newSub.nIndex = sub.nIndex;
        // If conversion failed (or was skipped due to GSM restrictions), use original data
        newSub.cbSubfile = newData ? newSize : sub.cbSubfile;
        newSub.oNext = curPos + sizeof(SubfileHeader) + newSub.cbSubfile;

        fwrite(&newSub, sizeof(newSub), 1, out);
        fwrite(newData ? newData : wavData, newSub.cbSubfile, 1, out);

        printf("Sound %u/%u (Grp %u, Idx %u): %lu -> %lu bytes [%s]%s\n",
               i + 1, hdr.cSounds, sub.nGroup, sub.nIndex,
               (unsigned long)sub.cbSubfile, (unsigned long)newSub.cbSubfile,
               newData ? modeName : "RAW (Skipped)",
               newData ? "" : " !");

        free(wavData);
        if (newData) free(newData);

        nextOffset = sub.oNext;
    }

    fclose(in);
    fclose(out);
    printf("Rebuilt %s successfully.\n", argv[3]);
    return 0;
}
