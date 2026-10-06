package sfx

import "encoding/binary"

// EncodeWAV 将整型采样（多声道按交错排列）编码为标准 PCM WAV 字节流。
// 输出为纯函数：相同输入必然得到逐字节一致的输出。
// 8 位按 WAV 规范转为无符号，16/24/32 位为小端有符号。
func EncodeWAV(samples []int32, f Format) []byte {
	bytesPerSample := f.BitDepth / 8
	blockAlign := f.Channels * bytesPerSample
	dataSize := len(samples) * bytesPerSample

	buf := make([]byte, 0, 44+dataSize)
	put := func(b []byte) { buf = append(buf, b...) }
	u32 := func(v uint32) {
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], v)
		put(b[:])
	}
	u16 := func(v uint16) {
		var b [2]byte
		binary.LittleEndian.PutUint16(b[:], v)
		put(b[:])
	}

	put([]byte("RIFF"))
	u32(uint32(36 + dataSize))
	put([]byte("WAVE"))
	put([]byte("fmt "))
	u32(16) // fmt 块大小
	u16(1)  // PCM
	u16(uint16(f.Channels))
	u32(uint32(f.SampleRate))
	u32(uint32(f.SampleRate * blockAlign)) // 字节率
	u16(uint16(blockAlign))
	u16(uint16(f.BitDepth))
	put([]byte("data"))
	u32(uint32(dataSize))

	for _, s := range samples {
		switch f.BitDepth {
		case 8:
			buf = append(buf, byte((s>>24)+128))
		case 16:
			u16(uint16(int16(s >> 16)))
		case 24:
			v := s >> 8
			buf = append(buf, byte(v), byte(v>>8), byte(v>>16))
		case 32:
			u32(uint32(s))
		}
	}
	return buf
}
