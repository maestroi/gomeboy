//go:build !android

package glfw

import (
	"image"
	"image/color"
	"image/draw"
	"strings"
	"time"

	"github.com/go-gl/gl/v4.6-core/gl"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

var (
	osdBG = color.RGBA{0, 0, 0, 200}
	osdFG = color.RGBA{255, 255, 255, 255}
)

const osdVertexShader = `#version 460 core
layout (location = 0) in vec2 pos;
layout (location = 1) in vec2 uv;
out vec2 fragUV;
uniform vec2 screenSize;
uniform vec2 quadSize;
uniform vec2 quadPos;
void main() {
    vec2 pixel = quadPos + pos * quadSize;
    vec2 ndc = (pixel / screenSize) * 2.0 - 1.0;
    gl_Position = vec4(ndc.x, -ndc.y, 0.0, 1.0);
    fragUV = uv;
}
` + "\x00"

const osdFragmentShader = `#version 460 core
in vec2 fragUV;
out vec4 outColor;
uniform sampler2D tex;
void main() {
    outColor = texture(tex, fragUV);
}
` + "\x00"

// osd renders text panels on top of the emulator framebuffer. Short status
// messages expire automatically; persistent, multiline panels are used by
// the in-window GLFW menu. Text is rasterized with basicfont and uploaded as
// a single texture so the desktop driver stays dependency-light.
type osd struct {
	program, vao, vbo, texture uint32
	message                    string
	expiresAt                  time.Time
	persistent                 bool
	texW, texH                 int32
}

func newOSD() *osd {
	o := &osd{program: compileOSDProgram()}

	verts := []float32{
		0, 0, 0, 0,
		1, 0, 1, 0,
		1, 1, 1, 1,
		0, 0, 0, 0,
		1, 1, 1, 1,
		0, 1, 0, 1,
	}
	gl.GenVertexArrays(1, &o.vao)
	gl.GenBuffers(1, &o.vbo)
	gl.BindVertexArray(o.vao)
	gl.BindBuffer(gl.ARRAY_BUFFER, o.vbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(verts)*4, gl.Ptr(verts), gl.STATIC_DRAW)
	gl.VertexAttribPointerWithOffset(0, 2, gl.FLOAT, false, 4*4, 0)
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointerWithOffset(1, 2, gl.FLOAT, false, 4*4, 2*4)
	gl.EnableVertexAttribArray(1)

	gl.GenTextures(1, &o.texture)
	gl.BindTexture(gl.TEXTURE_2D, o.texture)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)

	return o
}

// Show displays a short-lived status message.
func (o *osd) Show(text string, dur time.Duration) {
	o.persistent = false
	o.expiresAt = time.Now().Add(dur)
	o.render(text)
}

// Set displays a persistent panel until Hide or another Show/Set call.
func (o *osd) Set(text string) {
	if text == "" {
		o.Hide()
		return
	}
	o.persistent = true
	o.expiresAt = time.Time{}
	o.render(text)
}

func (o *osd) Hide() {
	o.message = ""
	o.persistent = false
	o.expiresAt = time.Time{}
}

func (o *osd) render(text string) {
	o.message = text
	lines := strings.Split(text, "\n")
	face := basicfont.Face7x13

	width := 0
	for _, line := range lines {
		if w := font.MeasureString(face, line).Ceil(); w > width {
			width = w
		}
	}

	const padding = 8
	const lineHeight = 15
	w := width + padding
	h := len(lines)*lineHeight + padding
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(osdBG), image.Point{}, draw.Src)

	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(osdFG),
		Face: face,
	}
	for i, line := range lines {
		d.Dot = fixed.P(4, 4+i*lineHeight+11)
		d.DrawString(line)
	}

	o.texW, o.texH = int32(w), int32(h)
	gl.BindTexture(gl.TEXTURE_2D, o.texture)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, o.texW, o.texH, 0, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(img.Pix))
}

// Draw renders the current panel into the top-left corner of a screenW x
// screenH viewport with an 8px margin.
func (o *osd) Draw(screenW, screenH int32) {
	if o.message == "" {
		return
	}
	if !o.persistent && time.Now().After(o.expiresAt) {
		return
	}

	gl.UseProgram(o.program)
	gl.Uniform2f(gl.GetUniformLocation(o.program, gl.Str("screenSize\x00")), float32(screenW), float32(screenH))
	gl.Uniform2f(gl.GetUniformLocation(o.program, gl.Str("quadSize\x00")), float32(o.texW), float32(o.texH))
	gl.Uniform2f(gl.GetUniformLocation(o.program, gl.Str("quadPos\x00")), 8, 8)

	gl.ActiveTexture(gl.TEXTURE0)
	gl.BindTexture(gl.TEXTURE_2D, o.texture)
	gl.Uniform1i(gl.GetUniformLocation(o.program, gl.Str("tex\x00")), 0)

	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)

	gl.BindVertexArray(o.vao)
	gl.DrawArrays(gl.TRIANGLES, 0, 6)

	gl.Disable(gl.BLEND)
}

func compileOSDProgram() uint32 {
	compile := func(src string, shaderType uint32) uint32 {
		shader := gl.CreateShader(shaderType)
		csrc, free := gl.Strs(src)
		gl.ShaderSource(shader, 1, csrc, nil)
		free()
		gl.CompileShader(shader)
		return shader
	}
	vs := compile(osdVertexShader, gl.VERTEX_SHADER)
	fs := compile(osdFragmentShader, gl.FRAGMENT_SHADER)
	prog := gl.CreateProgram()
	gl.AttachShader(prog, vs)
	gl.AttachShader(prog, fs)
	gl.LinkProgram(prog)
	gl.DeleteShader(vs)
	gl.DeleteShader(fs)
	return prog
}
