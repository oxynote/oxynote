package strutil

import "crypto/rand"

// _nanoidAlphabet is the default alphabet used by nanoid.
// This matches the Node.js nanoid default: A-Za-z0-9_-.
const _nanoidAlphabet = "useandom-26T198340PX75pxJACKVERYMINDBUSHWOLF_GQZbfghjklqvwyzrict"

// _nanoidLength is the default length of nanoid IDs (21 characters).
const _nanoidLength = 21

// NanoID generates a nanoid-compatible unique identifier. The output is 1:1
// compatible with the Node.js nanoid() function using default settings.
func NanoID() string {
	bytes := make([]byte, _nanoidLength)

	_, err := rand.Read(bytes)
	if err != nil {
		// NOCOV: crypto/rand failures cannot be simulated in tests.
		panic(err)
	}

	id := make([]byte, _nanoidLength)

	for i := range _nanoidLength {
		id[i] = _nanoidAlphabet[bytes[i]&63]
	}

	return string(id)
}
