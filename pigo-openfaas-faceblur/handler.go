// MIT License
//
// Copyright (c) 2019 Endre Simo
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package function

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/esimov/pigo/core"
	"github.com/esimov/stackblur-go"
	"github.com/fogleman/gg"
)

var dc *gg.Context

// FaceDetector struct contains Pigo face detector general settings.
type FaceDetector struct {
	cascadeFile  string
	minSize      int
	maxSize      int
	shiftFactor  float64
	scaleFactor  float64
	iouThreshold float64
}

// DetectionResult contains the coordinates of the detected faces and the base64 converted image.
type DetectionResult struct {
	Faces       []image.Rectangle
	ImageBase64 string
}

// Handle a serverless request
func Handle(req []byte) string {
	var (
		resp  DetectionResult
		rects []image.Rectangle
		data  []byte
		image []byte
	)

	if val, exists := os.LookupEnv("input_mode"); exists && val == "url" {
		inputURL := strings.TrimSpace(string(req))

		res, err := http.Get(inputURL)
		if err != nil {
			return fmt.Sprintf("Unable to download image file from URI: %s, status %v", inputURL, res.Status)
		}
		defer res.Body.Close()

		data, err = ioutil.ReadAll(res.Body)
		if err != nil {
			return fmt.Sprintf("Unable to read response body: %s", err)
		}
	} else {
		var decodeError error
		data, decodeError = base64.StdEncoding.DecodeString(string(req))
		if decodeError != nil {
			data = req
		}

		contentType := http.DetectContentType(req)
		if contentType != "image/jpeg" && contentType != "image/png" {
			return fmt.Sprintf("Only jpeg or png images, either raw uncompressed bytes or base64 encoded are acceptable inputs, you uploaded: %s", contentType)
		}
	}
	tmpfile, err := ioutil.TempFile("/tmp", "image")
	if err != nil {
		log.Fatalf("Unable to create temporary file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	_, err = io.Copy(tmpfile, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Sprintf("Unable to copy the source URI to the destionation file")
	}

	var output string
	query, err := url.ParseQuery(os.Getenv("Http_Query"))
	if err == nil {
		output = query.Get("output")
	}

	if val, exists := os.LookupEnv("output_mode"); exists {
		output = val
	}

	fd := NewFaceDetector("./data/facefinder", 20, 2000, 0.1, 1.1, 0.18)
	faces, err := fd.DetectFaces(tmpfile.Name())

	if err != nil {
		return fmt.Sprintf("Error on face detection: %v", err)
	}

	if output == "image" || output == "json_image" {
		var err error
		rects, image, err = fd.DrawFaces(data, faces)
		if err != nil {
			return fmt.Sprintf("Error creating image output: %s", err)
		}

		resp = DetectionResult{
			Faces:       rects,
			ImageBase64: base64.StdEncoding.EncodeToString(image),
		}
	}
	if output == "image" {
		return string(image)
	}

	j, err := json.Marshal(resp)
	if err != nil {
		return fmt.Sprintf("Error encoding output: %s", err)
	}

	// Return face rectangle coordinates
	return string(j)
}

// SubImager is a wrapper implementing the SubImage method from the image package.
type SubImager interface {
	SubImage(r image.Rectangle) image.Image
}

// NewFaceDetector initialises the constructor function.
func NewFaceDetector(cf string, minSize, maxSize int, shf, scf, iou float64) *FaceDetector {
	return &FaceDetector{
		cascadeFile:  cf,
		minSize:      minSize,
		maxSize:      maxSize,
		shiftFactor:  shf,
		scaleFactor:  scf,
		iouThreshold: iou,
	}
}

// DetectFaces run the detection algorithm over the provided source image.
func (fd *FaceDetector) DetectFaces(source string) ([]pigo.Detection, error) {
	src, err := pigo.GetImage(source)
	if err != nil {
		return nil, err
	}

	pixels := pigo.RgbToGrayscale(src)
	cols, rows := src.Bounds().Max.X, src.Bounds().Max.Y

	dc = gg.NewContext(cols, rows)
	dc.DrawImage(src, 0, 0)

	cParams := pigo.CascadeParams{
		MinSize:     fd.minSize,
		MaxSize:     fd.maxSize,
		ShiftFactor: fd.shiftFactor,
		ScaleFactor: fd.scaleFactor,
		ImageParams: pigo.ImageParams{
			Pixels: pixels,
			Rows:   rows,
			Cols:   cols,
			Dim:    cols,
		},
	}

	cascadeFile, err := ioutil.ReadFile(fd.cascadeFile)
	if err != nil {
		return nil, err
	}

	pigo := pigo.NewPigo()
	// Unpack the binary file. This will return the number of cascade trees,
	// the tree depth, the threshold and the prediction from tree's leaf nodes.
	classifier, err := pigo.Unpack(cascadeFile)
	if err != nil {
		return nil, err
	}

	// Run the classifier over the obtained leaf nodes and return the detection results.
	// The result contains quadruplets representing the row, column, scale and detection score.
	faces := classifier.RunCascade(cParams, 0)

	// Calculate the intersection over union (IoU) of two clusters.
	faces = classifier.ClusterDetections(faces, fd.iouThreshold)

	return faces, nil
}

// DrawFaces marks the detected faces with a circle in case isCircle is true, otherwise marks with a rectangle.
func (fd *FaceDetector) DrawFaces(srcImage []byte, faces []pigo.Detection) ([]image.Rectangle, []byte, error) {
	var (
		qThresh float32 = 5.0
		rects   []image.Rectangle
	)
	img, _, err := image.Decode(bytes.NewReader(srcImage))
	if err != nil {
		return nil, nil, err
	}

	for _, face := range faces {
		if face.Q > qThresh {
			rect := image.Rect(
				face.Col-face.Scale/2,
				face.Row-face.Scale/2,
				face.Col+face.Scale/2,
				face.Row+face.Scale/2,
			)
			rects = append(rects, rect)

			subImg := img.(SubImager).SubImage(rect)
			dim := subImg.Bounds().Max.X - subImg.Bounds().Min.X
			sf := int(round(float64(dim) * 0.1))

			blur := stackblur.Process(subImg, uint32(sf))

			x, y := face.Col-face.Scale/2, face.Row-face.Scale/2
			dc.DrawImage(blur, x, y)
		}
	}

	finalImg := dc.Image()
	filename := fmt.Sprintf("/tmp/%d.jpg", time.Now().UnixNano())

	output, err := os.OpenFile(filename, os.O_CREATE|os.O_RDWR, 0755)
	if err != nil {
		return nil, nil, err
	}
	defer os.Remove(filename)

	jpeg.Encode(output, finalImg, &jpeg.Options{Quality: 100})

	rf, err := ioutil.ReadFile(filename)
	return rects, rf, err
}

// round returns the nearest integer.
func round(f float64) float64 {
	return math.Floor(f + .5)
}
