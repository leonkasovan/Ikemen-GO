#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>
#include <string.h>
#include <sndfile.h>
#include <lame/lame.h>
#include <vorbis/vorbisenc.h>
#include <vorbis/vorbisfile.h>

// ---- SND headers ----
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

// ---- Memory-backed file for libsndfile ----
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
    (void)ptr; (void)user_data;
    return 0; // read-only
}
static sf_count_t mem_tell(void* user_data) {
    return ((MemFile*)user_data)->pos;
}

// ---- WAV -> MP3 (always 44.1kHz stereo, VBR) ----
unsigned char* wav_to_mp3(const unsigned char* wavData, size_t wavSize,
                          size_t* mp3SizeOut) {
    SF_VIRTUAL_IO vio = {
        mem_get_filelen, mem_seek, mem_read, mem_write, mem_tell
    };
    MemFile mem = { wavData, (sf_count_t)wavSize, 0 };

    SF_INFO sfinfo;
    SNDFILE* snd = sf_open_virtual(&vio, SFM_READ, &sfinfo, &mem);
    if (!snd) {
        fprintf(stderr, "libsndfile: failed to decode WAV\n");
        return NULL;
    }

    lame_t lame = lame_init();
    lame_set_in_samplerate(lame, sfinfo.samplerate);
    lame_set_out_samplerate(lame, 44100);
    lame_set_num_channels(lame, 2);
    lame_set_mode(lame, JOINT_STEREO);
    lame_set_VBR(lame, vbr_default);
    lame_set_VBR_quality(lame, 4); // 0=best, 9=smallest
    lame_set_quality(lame, 2);
    lame_init_params(lame);

    size_t maxMp3Size = wavSize;
    unsigned char* mp3Buffer = malloc(maxMp3Size);
    if (!mp3Buffer) {
        sf_close(snd);
        lame_close(lame);
        return NULL;
    }

    short pcm[8192 * 2];
    unsigned char* p = mp3Buffer;
    size_t total = 0;

    int read, encoded;
    while ((read = sf_readf_short(snd, pcm, 8192)) > 0) {
        if (sfinfo.channels == 2) {
            encoded = lame_encode_buffer_interleaved(lame, pcm, read, p, maxMp3Size - total);
        } else {
            encoded = lame_encode_buffer(lame, pcm, NULL, read, p, maxMp3Size - total);
        }
        if (encoded < 0) break;
        p += encoded;
        total += encoded;
    }

    encoded = lame_encode_flush(lame, p, maxMp3Size - total);
    if (encoded > 0) total += encoded;

    *mp3SizeOut = total;

    lame_close(lame);
    sf_close(snd);

    return mp3Buffer;
}

// ---- WAV -> ADPCM WAV ----
unsigned char* wav_to_adpcm(const unsigned char* wavData, size_t wavSize,
                            size_t* outSize) {
    SF_VIRTUAL_IO vio = {
        mem_get_filelen, mem_seek, mem_read, mem_write, mem_tell
    };
    MemFile mem = { wavData, (sf_count_t)wavSize, 0 };

    SF_INFO sfinfo;
    SNDFILE* snd = sf_open_virtual(&vio, SFM_READ, &sfinfo, &mem);
    if (!snd) {
        fprintf(stderr, "libsndfile: failed to decode WAV\n");
        return NULL;
    }

    // Prepare temporary file for ADPCM output
    const char* tmpName = "tmp_adpcm.wav";
    SF_INFO outInfo = sfinfo;
    outInfo.format = SF_FORMAT_WAV | SF_FORMAT_IMA_ADPCM;
    SNDFILE* out = sf_open(tmpName, SFM_WRITE, &outInfo);
    if (!out) {
        sf_close(snd);
        fprintf(stderr, "libsndfile: cannot open ADPCM encoder\n");
        return NULL;
    }

    short pcm[8192 * 2];
    int read;
    while ((read = sf_readf_short(snd, pcm, 8192)) > 0) {
        sf_writef_short(out, pcm, read);
    }

    sf_close(out);
    sf_close(snd);

    // Read back into memory
    FILE* tmp = fopen(tmpName, "rb");
    fseek(tmp, 0, SEEK_END);
    long size = ftell(tmp);
    fseek(tmp, 0, SEEK_SET);
    unsigned char* buf = malloc(size);
    fread(buf, 1, size, tmp);
    fclose(tmp);
    remove(tmpName);

    *outSize = size;
    return buf;
}

// ---- WAV -> OGG (Vorbis) ----
unsigned char* wav_to_ogg(const unsigned char* wavData, size_t wavSize,
                          size_t* outSize) {
    SF_VIRTUAL_IO vio = {
        mem_get_filelen, mem_seek, mem_read, mem_write, mem_tell
    };
    MemFile mem = { wavData, (sf_count_t)wavSize, 0 };

    SF_INFO sfinfo;
    SNDFILE* snd = sf_open_virtual(&vio, SFM_READ, &sfinfo, &mem);
    if (!snd) {
        fprintf(stderr, "libsndfile: failed to decode WAV\n");
        return NULL;
    }

    // Prepare temporary file for OGG output
    const char* tmpName = "tmp_ogg.ogg";
    SF_INFO outInfo = sfinfo;
    outInfo.format = SF_FORMAT_OGG | SF_FORMAT_VORBIS;
    SNDFILE* out = sf_open(tmpName, SFM_WRITE, &outInfo);
    if (!out) {
        sf_close(snd);
        fprintf(stderr, "libsndfile: cannot open OGG encoder\n");
        return NULL;
    }

    short pcm[8192 * 2];
    int read;
    while ((read = sf_readf_short(snd, pcm, 8192)) > 0) {
        sf_writef_short(out, pcm, read);
    }

    sf_close(out);
    sf_close(snd);

    // Read back into memory
    FILE* tmp = fopen(tmpName, "rb");
    fseek(tmp, 0, SEEK_END);
    long size = ftell(tmp);
    fseek(tmp, 0, SEEK_SET);
    unsigned char* buf = malloc(size);
    fread(buf, 1, size, tmp);
    fclose(tmp);
    remove(tmpName);

    *outSize = size;
    return buf;
}

// ---- Main Rebuilder ----
int main(int argc, char** argv) {
    if (argc < 4) {
        printf("Usage:\n");
        printf("  %s --mp3   input.snd output.snd\n", argv[0]);
        printf("  %s --adpcm input.snd output.snd\n", argv[0]);
        printf("  %s --ogg   input.snd output.snd\n", argv[0]);
        return 1;
    }

    int useMp3 = (strcmp(argv[1], "--mp3") == 0);
    int useAdpcm = (strcmp(argv[1], "--adpcm") == 0);
    int useOgg = (strcmp(argv[1], "--ogg") == 0);

    if (!useMp3 && !useAdpcm && !useOgg) {
        fprintf(stderr, "Unknown mode: %s (use --mp3, --adpcm or --ogg)\n", argv[1]);
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
        fread(wavData, sub.cbSubfile, 1, in);

        size_t newSize = 0;
        unsigned char* newData = NULL;

        if (useMp3) {
            newData = wav_to_mp3(wavData, sub.cbSubfile, &newSize);
        } else if (useAdpcm) {
            newData = wav_to_adpcm(wavData, sub.cbSubfile, &newSize);
        } else if (useOgg) {
            newData = wav_to_ogg(wavData, sub.cbSubfile, &newSize);
        }

        long curPos = ftell(out);
        SubfileHeader newSub;
        newSub.nGroup = sub.nGroup;
        newSub.nIndex = sub.nIndex;
        newSub.cbSubfile = newData ? newSize : sub.cbSubfile;
        newSub.oNext = curPos + sizeof(SubfileHeader) + newSub.cbSubfile;

        fwrite(&newSub, sizeof(newSub), 1, out);
        fwrite(newData ? newData : wavData, newSub.cbSubfile, 1, out);

        printf("Rebuilt %s %u/%u (Group %u, Index %u): %lu -> %lu bytes (%s)\n", argv[2],
               i + 1, hdr.cSounds, sub.nGroup, sub.nIndex,
               (unsigned long)sub.cbSubfile, (unsigned long)newSub.cbSubfile,
               useMp3 ? "MP3" : (useAdpcm ? "ADPCM WAV" : "OGG"));

        free(wavData);
        free(newData);
        nextOffset = sub.oNext;
    }

    fclose(in);
    fclose(out);
    printf("Rebuilt %s in %s mode\n", argv[3], useMp3 ? "MP3" : (useAdpcm ? "ADPCM WAV" : "OGG"));
    return 0;
}