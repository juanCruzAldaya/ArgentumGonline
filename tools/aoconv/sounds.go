package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Sonido: los WAV de Argentum, recortados a lo que este juego usa.
//
// The original ships 223 WAV files and 72 MP3 tracks, 227 MB of audio for a
// game whose whole web client is 37 MB. Shipping it wholesale would have
// doubled what a player downloads before seeing a tile, and most of it is for
// systems this game does not have — fishing, mining, smithing, NPC merchants,
// rain, doors.
//
// So this converts a list instead of a directory: the handful of sounds the
// combat and item code can actually cause, plus the WAV every spell names in
// Hechizos.dat. Twenty-two files, thirty-two seconds. Downmixed to mono and
// resampled to 22 kHz, which is what half of them already were — the 44 kHz
// stereo ones are a swing and a splash, not music — and then Godot's own QOA
// compression takes the result to about 0.4 MB in the exported pack.
//
// Music is not converted at all, only copied: it is served over HTTP by the
// game server and fetched by the client if the player wants it, so it never
// enters the pack. See the -music-out flag and the client's audio.gd.

// The sounds the game itself plays, by the number the original files them
// under. These are Declares.bas's own SND_* constants — the server source's,
// not the client's — because in Argentum it is the server that decides a sound
// happened and tells everyone nearby.
//
// This game does not send a sound message at all: the client already receives
// the combat event, the spell event and the use result, so it knows what
// happened and plays the sound itself. The list is duplicated in the client's
// audio.gd, which is the one place that maps an event to one of these numbers.
var gameSounds = map[int]string{
	2:  "SND_SWING: el golpe que no entra",
	10: "SND_IMPACTO: el golpe que entra",
	12: "SND_IMPACTO2: el segundo impacto",
	11: "SND_USERMUERTE: alguien muere",
	37: "SND_ESCUDO: el escudo rechaza",
	46: "SND_BEBER: tomar una poción",
	23: "SND_PASOS1: un paso",
	24: "SND_PASOS2: el otro paso",
}

// sfxSampleRate is what everything is resampled to.
//
// 22050 rather than 44100 because it is the rate most of these files were
// authored at — the 44 kHz ones are the same sounds recorded later — and
// because doubling the rate of a 0,3 s swing buys nothing anybody can hear
// through a fight while doubling what it costs to download.
const sfxSampleRate = 22050

// convertSounds writes the curated sound set into outDir.
//
// spells is what decides most of the list: every distinct WAV named in
// Hechizos.dat, so adding a spell to the game brings its sound along without
// anybody editing a list here.
func convertSounds(audioDir, outDir string, spells map[int]Spell) error {
	wanted := map[int]bool{}
	for id := range gameSounds {
		wanted[id] = true
	}
	fromSpells := 0
	for _, spell := range spells {
		if spell.Wav > 0 && !wanted[spell.Wav] {
			wanted[spell.Wav] = true
			fromSpells++
		}
	}

	ids := make([]int, 0, len(wanted))
	for id := range wanted {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	var missing []int
	written, seconds, bytes := 0, 0.0, 0
	for _, id := range ids {
		raw, err := os.ReadFile(filepath.Join(audioDir, "WAV", fmt.Sprintf("%d.wav", id)))
		if err != nil {
			missing = append(missing, id)
			continue
		}
		samples, rate, err := decodeWav(raw)
		if err != nil {
			return fmt.Errorf("%d.wav: %w", id, err)
		}
		mono := resample(samples, rate, sfxSampleRate)
		out := encodeWav(mono, sfxSampleRate)
		if err := os.WriteFile(filepath.Join(outDir, fmt.Sprintf("%d.wav", id)), out, 0o644); err != nil {
			return err
		}
		written++
		seconds += float64(len(mono)) / sfxSampleRate
		bytes += len(out)
	}

	fmt.Printf("sonidos:      %d archivos (%d de hechizos), %.1f s, %.2f MB en %s\n",
		written, fromSpells, seconds, float64(bytes)/(1<<20), outDir)
	if len(missing) > 0 {
		// Not fatal: a sound that is not there is a sound that does not play,
		// and the game is entirely playable without it. Loud, though, because
		// the alternative is wondering later why casting is silent.
		fmt.Printf("  faltan en %s: %v\n", filepath.Join(audioDir, "WAV"), missing)
	}
	return nil
}

// copyMusic copies the two tracks the game asks for into outDir, under the
// names the client looks for.
//
// Copied and not converted: they are already MP3, Godot reads MP3 directly, and
// re-encoding would cost quality to save nothing. Which two is a choice about
// the game rather than about the data — see the map below.
//
// They land in the *server's* directory, not the client's. That is the whole
// point: 4,8 MB inside the exported pack would be 4,8 MB every visitor
// downloads before playing, music on or off. Served over HTTP they are fetched
// once, by whoever wants them, and cached by the browser afterwards.
var musicTracks = map[string]string{
	// Ullathorpe, the town theme — 45 of the original's maps are wilderness and
	// this is the one that sounds like being somewhere safe. The camp is where
	// you wait between matches, which is the same feeling.
	"lobby": "4.mp3",
	// The field theme, the most used track in the whole game (45 maps). A
	// composed world is stitched out of 135 real maps, so no single map's
	// music is "correct" for it; this is the one the most ground was set to.
	"match": "3.mp3",
}

func copyMusic(audioDir, outDir string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	names := make([]string, 0, len(musicTracks))
	for name := range musicTracks {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		src := filepath.Join(audioDir, "MP3", musicTracks[name])
		raw, err := os.ReadFile(src)
		if err != nil {
			fmt.Printf("  falta la música %s (%s)\n", name, src)
			continue
		}
		dst := filepath.Join(outDir, name+".mp3")
		if err := os.WriteFile(dst, raw, 0o644); err != nil {
			return err
		}
		fmt.Printf("música:       %s -> %s (%.2f MB)\n",
			musicTracks[name], dst, float64(len(raw))/(1<<20))
	}
	return nil
}

// decodeWav reads a RIFF/PCM file and returns mono samples in [-1, 1] plus the
// sample rate.
//
// Only PCM, 8 or 16 bit: every file this converter is pointed at is one of
// those two, checked, and a decoder that guessed at the compressed formats
// nobody uses here would be code with no way to be wrong out loud. Anything
// else is an error rather than silence.
func decodeWav(raw []byte) ([]float64, int, error) {
	if len(raw) < 12 || string(raw[0:4]) != "RIFF" || string(raw[8:12]) != "WAVE" {
		return nil, 0, fmt.Errorf("no es un RIFF/WAVE")
	}

	var channels, bits, rate int
	var format int
	var data []byte

	// Chunk walk. There is no fixed layout: LIST and fact chunks turn up
	// between fmt and data often enough that assuming they do not is how a
	// parser reads the wrong bytes and produces noise instead of failing.
	for pos := 12; pos+8 <= len(raw); {
		id := string(raw[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(raw[pos+4 : pos+8]))
		pos += 8
		if size < 0 || pos+size > len(raw) {
			size = len(raw) - pos
		}
		switch id {
		case "fmt ":
			if size < 16 {
				return nil, 0, fmt.Errorf("fmt corto: %d bytes", size)
			}
			format = int(binary.LittleEndian.Uint16(raw[pos : pos+2]))
			channels = int(binary.LittleEndian.Uint16(raw[pos+2 : pos+4]))
			rate = int(binary.LittleEndian.Uint32(raw[pos+4 : pos+8]))
			bits = int(binary.LittleEndian.Uint16(raw[pos+14 : pos+16]))
		case "data":
			data = raw[pos : pos+size]
		}
		pos += size + size&1 // los chunks se alinean a par
	}

	if format != 1 {
		return nil, 0, fmt.Errorf("formato %d, solo PCM (1)", format)
	}
	if channels < 1 || rate <= 0 || data == nil {
		return nil, 0, fmt.Errorf("cabecera incompleta: ch=%d rate=%d data=%v", channels, rate, data != nil)
	}

	var frames []float64
	switch bits {
	case 8:
		// 8-bit WAV is unsigned, centred on 128 — the one place the format
		// changes its mind about signedness.
		for i := 0; i+channels <= len(data); i += channels {
			sum := 0.0
			for c := 0; c < channels; c++ {
				sum += (float64(data[i+c]) - 128) / 128
			}
			frames = append(frames, sum/float64(channels))
		}
	case 16:
		step := 2 * channels
		for i := 0; i+step <= len(data); i += step {
			sum := 0.0
			for c := 0; c < channels; c++ {
				sum += float64(int16(binary.LittleEndian.Uint16(data[i+2*c:]))) / 32768
			}
			frames = append(frames, sum/float64(channels))
		}
	default:
		return nil, 0, fmt.Errorf("%d bits por muestra, solo 8 y 16", bits)
	}
	return frames, rate, nil
}

// resample stretches or squeezes a mono signal to a new rate, linearly.
//
// Linear interpolation and no anti-aliasing filter, deliberately: the only
// downsampling that happens here is 44,1 kHz to 22 kHz on a handful of short
// impacts, where the aliasing lands in the same band as the impact's own
// clatter. A proper filter would be more code than the thing it improves.
func resample(in []float64, from, to int) []float64 {
	if from == to || len(in) == 0 {
		return in
	}
	ratio := float64(from) / float64(to)
	out := make([]float64, int(float64(len(in))/ratio))
	for i := range out {
		at := float64(i) * ratio
		left := int(at)
		frac := at - float64(left)
		right := left + 1
		if right >= len(in) {
			right = len(in) - 1
		}
		out[i] = in[left]*(1-frac) + in[right]*frac
	}
	return out
}

// encodeWav writes 16-bit mono PCM.
func encodeWav(samples []float64, rate int) []byte {
	const bits, channels = 16, 1
	dataLen := len(samples) * 2

	buf := make([]byte, 0, 44+dataLen)
	put32 := func(v uint32) { buf = binary.LittleEndian.AppendUint32(buf, v) }
	put16 := func(v uint16) { buf = binary.LittleEndian.AppendUint16(buf, v) }

	buf = append(buf, "RIFF"...)
	put32(uint32(36 + dataLen))
	buf = append(buf, "WAVE"...)
	buf = append(buf, "fmt "...)
	put32(16)
	put16(1) // PCM
	put16(channels)
	put32(uint32(rate))
	put32(uint32(rate * channels * bits / 8)) // byte rate
	put16(uint16(channels * bits / 8))        // block align
	put16(bits)
	buf = append(buf, "data"...)
	put32(uint32(dataLen))

	for _, s := range samples {
		// Clipped rather than normalised: a downmix of two channels that were
		// both near full scale can exceed it, and quietly rescaling the whole
		// file to fix three samples would make every sound quieter than the
		// original for no reason anybody asked for.
		if s > 1 {
			s = 1
		} else if s < -1 {
			s = -1
		}
		buf = binary.LittleEndian.AppendUint16(buf, uint16(int16(s*32767)))
	}
	return buf
}
