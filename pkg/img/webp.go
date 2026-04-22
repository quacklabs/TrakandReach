package img

import (
	"bytes"
	"image"
	_ "image/jpeg"
	_ "image/png"

	"github.com/chai2010/webp"
)

func ToWebP(data []byte, quality float32) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	err = webp.Encode(&buf, img, &webp.Options{Lossless: false, Quality: quality})
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
