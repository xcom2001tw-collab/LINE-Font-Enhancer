package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var u = syscall.NewLazyDLL("user32.dll")
var k = syscall.NewLazyDLL("kernel32.dll")
var g = syscall.NewLazyDLL("gdi32.dll")
var a = syscall.NewLazyDLL("advapi32.dll")
var shell = syscall.NewLazyDLL("shell32.dll")
var ole = syscall.NewLazyDLL("ole32.dll")
var dlg = syscall.NewLazyDLL("comdlg32.dll")

func call(d *syscall.LazyDLL, n string, v ...uintptr) uintptr {
	r, _, _ := d.NewProc(n).Call(v...)
	return r
}
func p(s string) uintptr           { x, _ := syscall.UTF16PtrFromString(s); return uintptr(unsafe.Pointer(x)) }
func ptr(x unsafe.Pointer) uintptr { return uintptr(x) }

var base = filepath.Join(os.Getenv("LOCALAPPDATA"), "LINE-FreeType-Tool")
var installed = filepath.Join(base, "LINE_Font_Enhancer.exe")
var config = filepath.Join(base, "line-path.txt")
var logpath = filepath.Join(os.TempDir(), "LINE_Font_Enhancer_V20_RC5.log")
var hwnd, edit, autoc, disablec, instance, font, headfont uintptr
var dpi = 96
var controls []ctl
var bgBrush, whiteBrush uintptr
var uiTitle, uiSubtitle, footer uintptr

func rgb(r, g, b uintptr) uintptr { return r | g<<8 | b<<16 }
func fill(dc uintptr, r *rect, color uintptr) {
	b := call(g, "CreateSolidBrush", color)
	call(u, "FillRect", dc, ptr(unsafe.Pointer(r)), b)
	call(g, "DeleteObject", b)
}

type drawItem struct {
	Type, ID, Item, Action, State uint32
	Window, DC                    uintptr
	Area                          rect
	Data                          uintptr
}

func paintButton(d *drawItem) {
	primary := d.ID == 104
	bg, fg, edge := rgb(255, 255, 255), rgb(35, 56, 48), rgb(198, 216, 205)
	if primary {
		bg, fg, edge = rgb(0, 137, 75), rgb(255, 255, 255), rgb(0, 137, 75)
	}
	if d.State&1 != 0 {
		if primary {
			bg = rgb(0, 97, 54)
		} else {
			bg = rgb(222, 239, 229)
		}
	}
	if d.State&4 != 0 {
		fg = rgb(135, 145, 140)
	}
	fill(d.DC, &d.Area, rgb(245, 249, 246))
	pen := call(g, "CreatePen", 0, 1, edge)
	brush := call(g, "CreateSolidBrush", bg)
	op := call(g, "SelectObject", d.DC, pen)
	ob := call(g, "SelectObject", d.DC, brush)
	call(g, "RoundRect", d.DC, uintptr(d.Area.L), uintptr(d.Area.T), uintptr(d.Area.R), uintptr(d.Area.B), sc(9), sc(9))
	call(g, "SelectObject", d.DC, op)
	call(g, "SelectObject", d.DC, ob)
	call(g, "DeleteObject", pen)
	call(g, "DeleteObject", brush)
	of := call(g, "SelectObject", d.DC, font)
	call(g, "SetBkMode", d.DC, 1)
	call(g, "SetTextColor", d.DC, fg)
	text := textval(d.Window)
	call(u, "DrawTextW", d.DC, p(text), ^uintptr(0), ptr(unsafe.Pointer(&d.Area)), 0x25|0x800)
	call(g, "SelectObject", d.DC, of)
	if d.State&0x10 != 0 {
		r := d.Area
		r.L += 4
		r.T += 4
		r.R -= 4
		r.B -= 4
		call(u, "DrawFocusRect", d.DC, ptr(unsafe.Pointer(&r)))
	}
}

func trace(s string) {
	if fi, e := os.Stat(logpath); e == nil && fi.Size() > 256*1024 {
		_ = os.Truncate(logpath, 0)
	}
	f, e := os.OpenFile(logpath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e == nil {
		fmt.Fprintln(f, time.Now().Format(time.RFC3339), s)
		f.Close()
	}
}
func msg(s string) { call(u, "MessageBoxW", hwnd, p(s), p("LINE Font Enhancer V2.0"), 0x40) }
func valid(s string) bool {
	f, e := os.Stat(s)
	return e == nil && !f.IsDir() && strings.EqualFold(filepath.Base(s), "LINE.exe")
}
func saved() string {
	b, _ := os.ReadFile(config)
	return strings.TrimSpace(strings.TrimPrefix(string(b), "\ufeff"))
}
func running() bool {
	h, e := syscall.CreateToolhelp32Snapshot(2, 0)
	if e != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	var pe syscall.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for e = syscall.Process32First(h, &pe); e == nil; e = syscall.Process32Next(h, &pe) {
		if strings.EqualFold(syscall.UTF16ToString(pe.ExeFile[:]), "LINE.exe") {
			return true
		}
	}
	return false
}
func find() string {
	if s := saved(); valid(s) {
		return s
	}
	best := ""
	var t time.Time
	for _, root := range []string{filepath.Join(os.Getenv("LOCALAPPDATA"), "LINE"), filepath.Join(os.Getenv("ProgramFiles"), "LINE"), filepath.Join(os.Getenv("ProgramFiles(x86)"), "LINE")} {
		filepath.WalkDir(root, func(path string, d os.DirEntry, e error) error {
			if e != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if strings.EqualFold(d.Name(), "LINE.exe") {
				if _, e := os.Stat(filepath.Join(filepath.Dir(path), "Qt6Core.dll")); e == nil {
					if i, e := d.Info(); e == nil && i.ModTime().After(t) {
						best = path
						t = i.ModTime()
					}
				}
			}
			return nil
		})
	}
	return best
}
func launch(s string, mode int) error {
	if !valid(s) {
		return errors.New("找不到 LINE.exe，請指定正確位置。")
	}
	if running() {
		return errors.New("LINE 已在執行，請從系統匣完全結束後再試。")
	}
	cmd := exec.Command(s)
	cmd.Dir = filepath.Dir(s)
	cmd.Env = modeEnv(mode)
	return cmd.Start()
}
func folder(cs uintptr) string {
	b := make([]uint16, 32768)
	if call(shell, "SHGetFolderPathW", 0, cs, 0, 0, ptr(unsafe.Pointer(&b[0]))) != 0 {
		return ""
	}
	return syscall.UTF16ToString(b)
}

type guid struct {
	A    uint32
	B, C uint16
	D    [8]byte
}

var cls = guid{0x00021401, 0, 0, [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
var iid = guid{0x000214f9, 0, 0, [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
var pfid = guid{0x0000010b, 0, 0, [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}

func com(o uintptr, idx int, args ...uintptr) uintptr {
	v := *(*uintptr)(unsafe.Pointer(o))
	f := *(*uintptr)(unsafe.Pointer(v + uintptr(idx)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := syscall.SyscallN(f, append([]uintptr{o}, args...)...)
	return r
}
func ok(hr uintptr) bool { return int32(hr) >= 0 }
func linkobj() (uintptr, uintptr, error) {
	var o, f uintptr
	hr := call(ole, "CoCreateInstance", ptr(unsafe.Pointer(&cls)), 0, 1, ptr(unsafe.Pointer(&iid)), ptr(unsafe.Pointer(&o)))
	if !ok(hr) {
		return 0, 0, fmt.Errorf("捷徑 COM: %x", hr)
	}
	if !ok(com(o, 0, ptr(unsafe.Pointer(&pfid)), ptr(unsafe.Pointer(&f)))) {
		com(o, 2)
		return 0, 0, errors.New("無法取得捷徑介面")
	}
	return o, f, nil
}
func linkread(path string) (string, string, error) {
	o, f, e := linkobj()
	if e != nil {
		return "", "", e
	}
	defer com(o, 2)
	defer com(f, 2)
	if !ok(com(f, 5, p(path), 0)) {
		return "", "", errors.New("無法讀取捷徑")
	}
	b := make([]uint16, 32768)
	c := make([]uint16, 32768)
	if !ok(com(o, 3, ptr(unsafe.Pointer(&b[0])), uintptr(len(b)), 0, 4)) {
		return "", "", errors.New("無法取得捷徑目標")
	}
	com(o, 10, ptr(unsafe.Pointer(&c[0])), uintptr(len(c)))
	return syscall.UTF16ToString(b), syscall.UTF16ToString(c), nil
}
func owned(path string) bool {
	t, arg, e := linkread(path)
	return e == nil && ((strings.EqualFold(t, installed) && arg == "--launch") || (strings.EqualFold(filepath.Base(t), "wscript.exe") && strings.EqualFold(strings.Trim(arg, "\""), filepath.Join(base, "Launch.vbs"))))
}
func linkwrite(path, line string) error {
	if _, e := os.Stat(path); e == nil && !owned(path) {
		return errors.New("已有非本工具捷徑：" + path)
	}
	o, f, e := linkobj()
	if e != nil {
		return e
	}
	defer com(o, 2)
	defer com(f, 2)
	for _, hr := range []uintptr{com(o, 20, p(installed)), com(o, 11, p("--launch")), com(o, 9, p(base)), com(o, 17, p(line), 0), com(o, 7, p("LINE FreeType font rendering"))} {
		if !ok(hr) {
			return errors.New("捷徑設定失敗")
		}
	}
	if !ok(com(f, 6, p(path), 1)) {
		return errors.New("捷徑儲存失敗")
	}
	return nil
}

const runkey = `Software\Microsoft\Windows\CurrentVersion\Run`

func regopen() (syscall.Handle, error) {
	var h syscall.Handle
	e := syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, (*uint16)(unsafe.Pointer(p(runkey))), 0, 0x2001f, &h)
	return h, e
}

type item struct {
	Kind string
	Name string
	Type uint32
	Data []byte
}

var backup = filepath.Join(base, "native-startup.json")

func load() ([]item, error) {
	b, e := os.ReadFile(backup)
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	var v []item
	e = json.Unmarshal(b, &v)
	return v, e
}
func save(v []item) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(backup, b, 0600)
}
func regvalue(h syscall.Handle, n string) (uint32, []byte, error) {
	var typ, l uint32
	e := syscall.RegQueryValueEx(h, (*uint16)(unsafe.Pointer(p(n))), nil, &typ, nil, &l)
	if e != nil {
		return typ, nil, e
	}
	b := make([]byte, l+2)
	e = syscall.RegQueryValueEx(h, (*uint16)(unsafe.Pointer(p(n))), nil, &typ, &b[0], &l)
	return typ, b[:l], e
}
func command(b []byte) string {
	v := make([]uint16, len(b)/2)
	for i := range v {
		v[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	return syscall.UTF16ToString(v)
}
func isline(s, line string) bool {
	s = strings.TrimSpace(s)
	t := ""
	if strings.HasPrefix(s, "\"") {
		if i := strings.Index(s[1:], "\""); i >= 0 {
			t = s[1 : i+1]
		}
	} else {
		if i := strings.Index(strings.ToLower(s), ".exe"); i >= 0 {
			t = s[:i+4]
		}
	}
	buf := make([]uint16, 32768)
	n := call(k, "ExpandEnvironmentStringsW", p(t), ptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n > 0 && n <= uintptr(len(buf)) {
		t = syscall.UTF16ToString(buf)
	}
	return valid(t) && (strings.EqualFold(t, line) || strings.Contains(strings.ToLower(t), `\line\`))
}
func disableOriginal(line, start string) (int, error) {
	old, e := load()
	if e != nil {
		return 0, e
	}
	h, e := regopen()
	if e != nil {
		return 0, e
	}
	defer syscall.RegCloseKey(h)
	var pending []item
	for i := 0; ; i++ {
		b := make([]uint16, 16384)
		n := uint32(len(b))
		r := call(a, "RegEnumValueW", uintptr(h), uintptr(i), ptr(unsafe.Pointer(&b[0])), ptr(unsafe.Pointer(&n)), 0, 0, 0, 0)
		if r == 259 {
			break
		}
		if r != 0 {
			return 0, fmt.Errorf("讀取啟動項目失敗 %d", r)
		}
		name := syscall.UTF16ToString(b[:n])
		typ, data, e := regvalue(h, name)
		if e == nil && (typ == 1 || typ == 2) && isline(command(data), line) {
			pending = append(pending, item{"Run", name, typ, data})
		}
	}
	entries, _ := os.ReadDir(start)
	for _, v := range entries {
		if !strings.EqualFold(filepath.Ext(v.Name()), ".lnk") {
			continue
		}
		path := filepath.Join(start, v.Name())
		t, _, e := linkread(path)
		if e == nil && isline("\""+t+"\"", line) {
			b, e := os.ReadFile(path)
			if e != nil {
				return 0, e
			}
			pending = append(pending, item{"Link", path, 0, b})
		}
	}
	for _, v := range pending {
		exists := false
		for _, o := range old {
			if o.Kind == v.Kind && strings.EqualFold(o.Name, v.Name) {
				exists = true
				if string(o.Data) != string(v.Data) {
					return 0, errors.New("已有不同版本的啟動備份，請先還原。")
				}
			}
		}
		if !exists {
			old = append(old, v)
		}
	}
	if len(pending) == 0 {
		return 0, nil
	}
	if e = save(old); e != nil {
		return 0, e
	}
	count := 0
	for _, v := range pending {
		if v.Kind == "Run" {
			r := call(a, "RegDeleteValueW", uintptr(h), p(v.Name))
			if r != 0 {
				return count, fmt.Errorf("停用失敗 %d，備份已保留", r)
			}
		} else if e = os.Remove(v.Name); e != nil {
			return count, e
		}
		count++
	}
	return count, nil
}
func restore() (int, error) {
	old, e := load()
	if e != nil {
		return 0, e
	}
	if len(old) == 0 {
		return 0, nil
	}
	h, e := regopen()
	if e != nil {
		return 0, e
	}
	defer syscall.RegCloseKey(h)
	var rem []item
	count := 0
	for _, v := range old {
		good := false
		if v.Kind == "Run" {
			_, b, e := regvalue(h, v.Name)
			if e == nil {
				good = string(b) == string(v.Data)
			} else if e == syscall.ERROR_FILE_NOT_FOUND && len(v.Data) > 0 {
				good = call(a, "RegSetValueExW", uintptr(h), p(v.Name), 0, uintptr(v.Type), ptr(unsafe.Pointer(&v.Data[0])), uintptr(len(v.Data))) == 0
			}
		} else if v.Kind == "Link" {
			b, e := os.ReadFile(v.Name)
			if e == nil {
				good = string(b) == string(v.Data)
			} else if os.IsNotExist(e) {
				good = os.WriteFile(v.Name, v.Data, 0600) == nil
			}
		}
		if good {
			count++
		} else {
			rem = append(rem, v)
		}
	}
	if e = save(rem); e != nil {
		return count, e
	}
	if len(rem) > 0 {
		return count, errors.New("部分項目有衝突，未覆寫，備份已保留。")
	}
	return count, nil
}
func checked(h uintptr) bool { return call(u, "SendMessageW", h, 0xf0, 0, 0) == 1 }
func textval(h uintptr) string {
	n := call(u, "GetWindowTextLengthW", h)
	b := make([]uint16, n+1)
	call(u, "GetWindowTextW", h, ptr(unsafe.Pointer(&b[0])), n+1)
	return strings.TrimSpace(syscall.UTF16ToString(b))
}
func applySettings() error {
	line := textval(edit)
	if !valid(line) {
		return errors.New("請指定 LINE.exe")
	}
	desk, start := folder(0x10), folder(7)
	if desk == "" || start == "" {
		return errors.New("無法取得 Windows 捷徑資料夾")
	}
	if e := os.MkdirAll(base, 0700); e != nil {
		return e
	}
	self, e := os.Executable()
	if e != nil {
		return e
	}
	if !strings.EqualFold(self, installed) {
		b, e := os.ReadFile(self)
		if e != nil {
			return e
		}
		tmp := installed + ".new"
		if e = os.WriteFile(tmp, b, 0700); e != nil {
			return e
		}
		if e = os.Rename(tmp, installed); e != nil {
			return e
		}
	}
	if e = os.WriteFile(filepath.Join(base, "render-mode.txt"), []byte(fmt.Sprint(selectedMode())), 0600); e != nil {
		return e
	}
	if e = os.WriteFile(config, []byte(line), 0600); e != nil {
		return e
	}
	if e = linkwrite(filepath.Join(desk, "LINE FreeType.lnk"), line); e != nil {
		return e
	}
	sp := filepath.Join(start, "LINE FreeType.lnk")
	if checked(autoc) {
		if e = linkwrite(sp, line); e != nil {
			return e
		}
		if checked(disablec) {
			n, e := disableOriginal(line, start)
			if e != nil {
				return e
			}
			trace(fmt.Sprintf("disabled %d original startup entries", n))
			return nil
		}
	} else if owned(sp) {
		if e = os.Remove(sp); e != nil {
			return e
		}
	}
	n, e := restore()
	if e != nil {
		return e
	}
	trace(fmt.Sprintf("restored %d startup entries", n))
	return nil
}
func undo() error {
	for _, id := range []uintptr{0x10, 7} {
		d := folder(id)
		if d == "" {
			return errors.New("無法取得捷徑資料夾")
		}
		path := filepath.Join(d, "LINE FreeType.lnk")
		if owned(path) {
			if e := os.Remove(path); e != nil {
				return e
			}
		}
	}
	n, e := restore()
	if e != nil {
		return e
	}
	call(u, "SendMessageW", autoc, 0xf1, 0, 0)
	call(u, "EnableWindow", disablec, 0)
	s := fmt.Sprintf("已移除本工具捷徑，還原 %d 筆本版備份。", n)
	if _, e := os.Stat(filepath.Join(base, "original-startup.clixml")); e == nil {
		s += "\n舊版 XML 備份仍保留，V2.0 不會修改它。請用 V1.1 還原舊版備份。"
	}
	msg(s)
	return nil
}

type rect struct{ L, T, R, B int32 }
type point struct{ X, Y int32 }
type message struct {
	H       uintptr
	M       uint32
	W, L    uintptr
	Time    uint32
	P       point
	Private uint32
}
type wc struct {
	Size, Style                        uint32
	Proc                               uintptr
	ClsExtra, WndExtra                 int32
	Instance, Icon, Cursor, Background uintptr
	Menu, Class                        *uint16
	SmallIcon                          uintptr
}
type ctl struct {
	h           uintptr
	x, y, w, ht int
	head        bool
}

func sc(v int) uintptr { return uintptr(v * dpi / 96) }
func makec(class, label string, style uint32, id, x, y, w, h int, head bool) uintptr {
	if class == "BUTTON" && style == 0x10000 {
		style |= 0x0b
	}
	r := call(u, "CreateWindowExW", 0, p(class), p(label), uintptr(style|0x50000000), sc(x), sc(y), sc(w), sc(h), hwnd, uintptr(id), instance, 0)
	controls = append(controls, ctl{r, x, y, w, h, head})
	return r
}
func fonts() {
	old, oldh := font, headfont
	font = call(g, "CreateFontW", uintptr(-int32(15*dpi/96)), 0, 0, 0, 500, 0, 0, 0, 1, 0, 0, 4, 0, p("Microsoft JhengHei UI"))
	headfont = call(g, "CreateFontW", uintptr(-int32(26*dpi/96)), 0, 0, 0, 700, 0, 0, 0, 1, 0, 0, 4, 0, p("Microsoft JhengHei UI"))
	for _, c := range controls {
		f := font
		if c.head {
			f = headfont
		}
		call(u, "SendMessageW", c.h, 0x30, f, 1)
		call(u, "MoveWindow", c.h, sc(c.x), sc(c.y), sc(c.w), sc(c.ht), 1)
	}
	if old != 0 {
		call(g, "DeleteObject", old)
	}
	if oldh != 0 {
		call(g, "DeleteObject", oldh)
	}
}

type ofn struct {
	Size                      uint32
	Owner, Instance           uintptr
	Filter, Custom            *uint16
	MaxCustom, FilterIndex    uint32
	File                      *uint16
	MaxFile                   uint32
	FileTitle                 *uint16
	MaxFileTitle              uint32
	Initial, Title            *uint16
	Flags                     uint32
	FileOffset, FileExtension uint16
	DefExt                    *uint16
	Cust, Hook                uintptr
	Template                  *uint16
	Reserved                  uintptr
	Reserved2, FlagsEx        uint32
}

func browse() {
	b := make([]uint16, 32768)
	f := []uint16{'L', 'I', 'N', 'E', '.', 'e', 'x', 'e', 0, 'L', 'I', 'N', 'E', '.', 'e', 'x', 'e', 0, 0}
	o := ofn{Owner: hwnd, Filter: &f[0], File: &b[0], MaxFile: uint32(len(b)), Flags: 0x1800 | 0x800}
	o.Size = uint32(unsafe.Sizeof(o))
	if call(dlg, "GetOpenFileNameW", ptr(unsafe.Pointer(&o))) != 0 {
		call(u, "SetWindowTextW", edit, p(syscall.UTF16ToString(b)))
	}
}
func wnd(h uintptr, m uint32, w, l uintptr) uintptr {
	switch m {
	case 0x2b:
		if l != 0 {
			d := (*drawItem)(unsafe.Pointer(l))
			if d.Type == 4 {
				paintButton(d)
				return 1
			}
		}
	case 0x138, 0x135, 0x133, 0x134:
		dc, child := w, l
		fg := rgb(38, 54, 47)
		if child == uiTitle || child == labels[1] {
			fg = rgb(0, 119, 66)
		}
		if child == uiSubtitle || child == note || child == footer {
			fg = rgb(83, 105, 94)
		}
		if m == 0x133 || m == 0x134 {
			call(g, "SetBkColor", dc, rgb(255, 255, 255))
			call(g, "SetTextColor", dc, fg)
			return whiteBrush
		}
		call(g, "SetBkMode", dc, 1)
		call(g, "SetTextColor", dc, fg)
		return bgBrush
	case 0x111:
		var e error
		switch w & 0xffff {
		case 101:
			browse()
			showSamples()
		case 102:
			showSamples()
		case 120, 121:
			if w>>16 == 1 {
				showSamples()
			}
		case 123:
			if !valid(textval(edit)) {
				e = errors.New("請指定 LINE.exe")
			} else if e = closeLine(textval(edit)); e == nil {
				e = launch(textval(edit), 0)
			}
		case 103:
			e = actualTrial()
		case 104:
			e = applyAndRestart()
		case 105:
			e = undoAndRestart()
		case 106:
			call(u, "EnableWindow", disablec, func() uintptr {
				if checked(autoc) {
					return 1
				}
				return 0
			}())
		case 107:
			call(u, "SetWindowTextW", edit, p(find()))
			showSamples()
		}
		if e != nil {
			trace(e.Error())
			msg(e.Error())
		}
		return 0
	case 0x2e0:
		dpi = int(w & 0xffff)
		r := (*rect)(unsafe.Pointer(l))
		call(u, "SetWindowPos", h, 0, uintptr(r.L), uintptr(r.T), uintptr(r.R-r.L), uintptr(r.B-r.T), 0x14)
		fonts()
		showSamples()
		return 0
	case 2:
		call(u, "PostQuitMessage", 0)
		return 0
	}
	return call(u, "DefWindowProcW", h, uintptr(m), w, l)
}
func main() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	trace("native launcher started")
	defer func() {
		if v := recover(); v != nil {
			trace(fmt.Sprint(v))
			msg(fmt.Sprint(v))
		}
	}()
	if len(os.Args) > 1 && os.Args[1] == "--launch" {
		if !running() {
			if e := launch(find(), savedMode()); e != nil {
				msg(e.Error())
			}
		}
		return
	}
	call(ole, "CoInitializeEx", 0, 2)
	defer call(ole, "CoUninitialize")
	call(u, "SetProcessDpiAwarenessContext", ^uintptr(3))
	dpi = int(call(u, "GetDpiForSystem"))
	if dpi == 0 {
		dpi = 96
	}
	instance = call(k, "GetModuleHandleW", 0)
	bgBrush = call(g, "CreateSolidBrush", rgb(245, 249, 246))
	whiteBrush = call(g, "CreateSolidBrush", rgb(255, 255, 255))
	defer call(g, "DeleteObject", bgBrush)
	defer call(g, "DeleteObject", whiteBrush)
	cl, _ := syscall.UTF16PtrFromString("LineFontEnhancerNative20")
	c := wc{Style: 3, Proc: syscall.NewCallback(wnd), Instance: instance, Icon: call(u, "LoadIconW", instance, 1), Cursor: call(u, "LoadCursorW", 0, 32512), Background: bgBrush, Class: cl}
	c.Size = uint32(unsafe.Sizeof(c))
	if call(u, "RegisterClassExW", ptr(unsafe.Pointer(&c))) == 0 {
		msg("無法註冊視窗類別")
		return
	}
	if err := initSamples(); err != nil {
		trace(err.Error())
		msg(err.Error())
		return
	}
	pc, _ := syscall.UTF16PtrFromString("LFEPreviewRC5")
	previewCallback = syscall.NewCallback(previewWnd)
	pcw := wc{Style: 3, Proc: previewCallback, Instance: instance, Cursor: call(u, "LoadCursorW", 0, 32512), Background: 6, Class: pc}
	pcw.Size = uint32(unsafe.Sizeof(pcw))
	pr, _, _ := u.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&pcw)))
	if pr == 0 {
		msg("無法建立預覽視窗類別")
		return
	}
	runtime.KeepAlive(pc)
	r := rect{R: int32(sc(860)), B: int32(sc(510))}
	call(u, "AdjustWindowRectExForDpi", ptr(unsafe.Pointer(&r)), 0x00ca0000, 0, 0, uint32ptr(dpi))
	ww, hh := r.R-r.L, r.B-r.T
	var wa rect
	call(u, "SystemParametersInfoW", 0x30, 0, ptr(unsafe.Pointer(&wa)), 0)
	hwnd = call(u, "CreateWindowExW", 0x40000, p("LineFontEnhancerNative20"), p("LINE Font Enhancer V2.0 RC5 | 字體優化工具"), 0x00ca0000, uintptr(wa.L+(wa.R-wa.L-ww)/2), uintptr(wa.T+(wa.B-wa.T-hh)/2), uintptr(ww), uintptr(hh), 0, 0, instance, 0)
	if hwnd == 0 {
		trace("CreateWindowEx failed")
		msg("無法建立視窗")
		return
	}
	uiTitle = makec("STATIC", "LINE Font Enhancer", 0, 0, 18, 10, 824, 34, true)
	uiSubtitle = makec("STATIC", "熟悉的 LINE，不一樣的質感。  |  FreeType Edition", 0, 0, 18, 46, 824, 22, false)
	edit = makec("EDIT", "", 0x00810080, 110, 18, 74, 660, 25, false)
	makec("BUTTON", "LINE 位置…", 0x10000, 101, 686, 72, 156, 29, false)
	makec("STATIC", "比較字體", 0, 0, 18, 112, 80, 24, false)
	fontChoice = makec("COMBOBOX", "", 0x00210003, 120, 104, 109, 242, 180, false)
	combo(fontChoice, names, 0)
	zoomChoice = makec("COMBOBOX", "", 0x00210003, 121, 360, 109, 150, 160, false)
	combo(zoomChoice, []string{"完整樣本", "100% 原始像素", "200% 局部", "400% 局部"}, 0)
	makec("BUTTON", "原廠實際試用", 0x10000, 123, 686, 108, 156, 29, false)
	for i := 0; i < 2; i++ {
		x := 18 + i*420
		labels[i] = makec("STATIC", "", 0, 0, x, 146, 404, 24, false)
		previews[i] = makec("LFEPreviewRC5", "", 0x00800000, 0, x, 173, 404, 202, false)
	}
	note = makec("STATIC", "", 0, 0, 18, 381, 824, 23, false)
	autoc = makec("BUTTON", "登入時自動啟動優化版", 0x10003, 106, 18, 410, 400, 25, false)
	disablec = makec("BUTTON", "停用原版自啟（可還原）", 0x10003, 108, 438, 410, 404, 25, false)
	call(u, "SendMessageW", disablec, 0xf1, 1, 0)
	if owned(filepath.Join(folder(7), "LINE FreeType.lnk")) {
		call(u, "SendMessageW", autoc, 0xf1, 1, 0)
	} else {
		call(u, "EnableWindow", disablec, 0)
	}
	makec("BUTTON", "FreeType 實際試用", 0x10000, 103, 18, 445, 220, 36, false)
	makec("BUTTON", "一鍵美化 LINE", 0x10000, 104, 250, 445, 360, 36, false)
	makec("BUTTON", "還原原廠設定", 0x10000, 105, 622, 445, 220, 36, false)
	footer = makec("STATIC", "V2.0 RC5  •  免費 / 離線 / 非官方  •  不修改 LINE 本體", 0, 0, 18, 487, 824, 20, false)

	fonts()
	call(u, "ShowWindow", hwnd, 5)
	call(u, "UpdateWindow", hwnd)
	call(u, "SetForegroundWindow", hwnd)
	trace("native window visible")
	call(u, "SetWindowTextW", edit, p(find()))
	trace("LINE discovery completed")
	showSamples()
	go cleanLegacyPreview()
	var m message
	for {
		r := int32(call(u, "GetMessageW", ptr(unsafe.Pointer(&m)), 0, 0, 0))
		if r <= 0 {
			break
		}
		if (m.H != hwnd && call(u, "IsChild", hwnd, m.H) == 0) || call(u, "IsDialogMessageW", hwnd, ptr(unsafe.Pointer(&m))) == 0 {
			call(u, "TranslateMessage", ptr(unsafe.Pointer(&m)))
			call(u, "DispatchMessageW", ptr(unsafe.Pointer(&m)))
		}
	}
	trace("native window closed")
}
func uint32ptr(v int) uintptr { return uintptr(v) }

// Only close verified LINE.exe instances from the current Windows session.
type lineProc struct {
	pid uint32
	h   syscall.Handle
}

func matchingProcesses(line string) ([]lineProc, error) {
	snap, e := syscall.CreateToolhelp32Snapshot(2, 0)
	if e != nil {
		return nil, e
	}
	defer syscall.CloseHandle(snap)
	var cur uint32
	if call(k, "ProcessIdToSessionId", call(k, "GetCurrentProcessId"), ptr(unsafe.Pointer(&cur))) == 0 {
		return nil, errors.New("無法確認目前 Windows 工作階段")
	}
	var list []lineProc
	cleanup := func() {
		for _, v := range list {
			syscall.CloseHandle(v.h)
		}
	}
	var pe syscall.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for e = syscall.Process32First(snap, &pe); e == nil; e = syscall.Process32Next(snap, &pe) {
		if !strings.EqualFold(syscall.UTF16ToString(pe.ExeFile[:]), "LINE.exe") {
			continue
		}
		var sess uint32
		if call(k, "ProcessIdToSessionId", uintptr(pe.ProcessID), ptr(unsafe.Pointer(&sess))) == 0 || sess != cur {
			continue
		}
		h, err := syscall.OpenProcess(0x101001, false, pe.ProcessID) // query limited, synchronize, terminate
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("無法存取 LINE 程序：%w", err)
		}
		b := make([]uint16, 32768)
		n := uint32(len(b))
		if call(k, "QueryFullProcessImageNameW", uintptr(h), 0, ptr(unsafe.Pointer(&b[0])), ptr(unsafe.Pointer(&n))) == 0 || !strings.EqualFold(filepath.Clean(syscall.UTF16ToString(b[:n])), filepath.Clean(line)) {
			syscall.CloseHandle(h)
			cleanup()
			return nil, errors.New("偵測到另一個 LINE 安裝位置，請先結束該版本或重新選擇正確執行檔。")
		}
		list = append(list, lineProc{pe.ProcessID, h})
	}
	return list, nil
}
func live(list []lineProc) []lineProc {
	var out []lineProc
	for _, v := range list {
		if call(k, "WaitForSingleObject", uintptr(v.h), 0) != 0 {
			out = append(out, v)
		}
	}
	return out
}
func waitExit(list []lineProc, duration time.Duration) []lineProc {
	until := time.Now().Add(duration)
	for {
		v := live(list)
		if len(v) == 0 || time.Now().After(until) {
			return v
		}
		time.Sleep(100 * time.Millisecond)
	}
}
func confirm(s string) bool {
	return call(u, "MessageBoxW", hwnd, p(s), p("LINE Font Enhancer V2.0"), 0x24|0x100) == 6
}

var closingIDs map[uint32]bool
var closeCallback = syscall.NewCallback(func(h, l uintptr) uintptr {
	var pid uint32
	call(u, "GetWindowThreadProcessId", h, ptr(unsafe.Pointer(&pid)))
	if closingIDs[pid] {
		call(u, "PostMessageW", h, 0x10, 0, 0)
	}
	return 1
})

func closeLine(line string) error {
	list, e := matchingProcesses(line)
	if e != nil {
		return e
	}
	defer func() {
		for _, v := range list {
			syscall.CloseHandle(v.h)
		}
	}()
	if len(list) == 0 {
		return nil
	}
	if !confirm("即將自動結束 LINE 並重新啟動。請確認沒有通話、傳檔或尚未送出的訊息。是否繼續？") {
		return errors.New("已取消，尚未套用設定。")
	}
	closingIDs = map[uint32]bool{}
	for _, v := range list {
		closingIDs[v.pid] = true
	}
	call(u, "EnumWindows", closeCallback, 0)
	closingIDs = nil
	remain := waitExit(list, 4*time.Second)
	if len(remain) > 0 {
		if !confirm("LINE 仍在背景執行（可能已縮至系統匣）。是否強制結束？強制結束可能中斷未完成的傳輸或尚未儲存的內容。選否則取消套用。") {
			return errors.New("已取消套用；LINE 可能已縮至系統匣。")
		}
		for _, v := range live(remain) {
			if call(k, "TerminateProcess", uintptr(v.h), 0) == 0 && len(live([]lineProc{v})) > 0 {
				return errors.New("無法結束 LINE，未套用設定。")
			}
		}
		if len(waitExit(remain, 3*time.Second)) > 0 {
			return errors.New("LINE 尚未完全結束，未套用設定。")
		}
	}
	if running() {
		return errors.New("LINE 已重新啟動或其他工作階段仍在使用，未套用設定。")
	}
	return nil
}
func applyAndRestart() error {
	line := textval(edit)
	if !valid(line) {
		return errors.New("請指定 LINE.exe")
	}

	if e := closeLine(line); e != nil {
		return e
	}
	if e := applySettings(); e != nil {
		return fmt.Errorf("套用未完整完成：%w\nLINE 可能已關閉，可從原版捷徑重新開啟。若有部分設定已變更，請使用還原設定。", e)
	}
	if e := launch(line, selectedMode()); e != nil {
		return fmt.Errorf("設定已套用，但 LINE 重新啟動失敗：%w\n請使用桌面 LINE FreeType 捷徑。", e)
	}
	msg("套用完成，已送出所選模式啟動指令。\n日後請使用桌面 LINE FreeType 捷徑。")
	return nil
}
func undoAndRestart() error {
	line := textval(edit)
	if !valid(line) {
		return errors.New("請指定 LINE.exe")
	}
	if !confirm("還原工具的捷徑及原廠自啟設定，並重新啟動原廠 LINE？") {
		return nil
	}
	if e := closeLine(line); e != nil {
		return e
	}
	if e := undo(); e != nil {
		return e
	}
	return launch(line, 0)
}
func actualTrial() error {
	line := textval(edit)
	if !valid(line) {
		return errors.New("請指定 LINE.exe")
	}

	if e := closeLine(line); e != nil {
		return e
	}
	return launch(line, selectedMode())
}

//go:embed sample_*.png
var sampleFiles embed.FS
var names = []string{"微軟正黑體", "金萱粉圓體", "標楷體", "新細明體", "Noto Sans TC Medium"}
var previews, labels [2]uintptr
var samples [5][2]image.Image
var frames [2][]byte
var frameW, frameH int
var previewClass, previewCallback uintptr
var fontChoice, zoomChoice, note uintptr
var mapping = [5]int{1, 2, 2, 1, 1}
var sampleIndex, zoomIndex int

func selectedMode() int { return 1 }
func savedMode() int    { return 1 }
func modeEnv(mode int) []string {
	var env []string
	for _, v := range os.Environ() {
		n := strings.ToUpper(v)
		if !strings.HasPrefix(n, "QT_QPA_PLATFORM=") && !strings.HasPrefix(n, "QT_QUICK_BACKEND=") {
			env = append(env, v)
		}
	}
	if mode == 1 {
		env = append(env, "QT_QPA_PLATFORM=windows:fontengine=freetype")
	}
	return env
}

type bitmapInfo struct {
	Size                   uint32
	Width, Height          int32
	Planes, Bits           uint16
	Compression, ImageSize uint32
	X, Y                   int32
	Used, Important, Color uint32
}

func combo(h uintptr, items []string, sel int) {
	for _, v := range items {
		call(u, "SendMessageW", h, 0x143, 0, p(v))
	}
	call(u, "SendMessageW", h, 0x14e, uintptr(sel), 0)
}
func initSamples() error {
	for i := range samples {
		for j := range samples[i] {
			data, e := sampleFiles.ReadFile(fmt.Sprintf("sample_%d_%d.png", i, j))
			if e != nil {
				return e
			}
			im, e := png.Decode(bytes.NewReader(data))
			if e != nil {
				return e
			}
			samples[i][j] = im
		}
	}
	return nil
}

// Paint directly from Go-owned pixel buffers. No HBITMAP ownership or DIB pointers.
func previewWnd(h uintptr, m uint32, w, l uintptr) uintptr {
	if m == 0x000f {
		var ps struct {
			DC              uintptr
			Erase           int32
			Paint           rect
			Restore, Update int32
			Reserved        [32]byte
		}
		dc, _, _ := u.NewProc("BeginPaint").Call(h, uintptr(unsafe.Pointer(&ps)))
		if dc != 0 {
			for i, ph := range previews {
				if ph == h && len(frames[i]) > 0 && frameW > 0 && frameH > 0 {
					info := bitmapInfo{Size: 40, Width: int32(frameW), Height: -int32(frameH), Planes: 1, Bits: 32}
					g.NewProc("StretchDIBits").Call(dc, 0, 0, uintptr(frameW), uintptr(frameH), 0, 0, uintptr(frameW), uintptr(frameH), uintptr(unsafe.Pointer(&frames[i][0])), uintptr(unsafe.Pointer(&info)), 0, 0x00CC0020)
					runtime.KeepAlive(frames[i])
					runtime.KeepAlive(&info)
				}
			}
		}
		u.NewProc("EndPaint").Call(h, uintptr(unsafe.Pointer(&ps)))
		return 0
	}
	return call(u, "DefWindowProcW", h, uintptr(m), w, l)
}
func showSamples() {
	if previews[0] == 0 || previews[1] == 0 {
		return
	}
	defer func() {
		if e := recover(); e != nil {
			trace(fmt.Sprintf("preview panic: %v", e))
			msg("預覽更新失敗，請查看診斷紀錄。")
		}
	}()
	si := int(call(u, "SendMessageW", fontChoice, 0x147, 0, 0))
	if si >= 0 && si < len(names) {
		sampleIndex = si
	}
	zi := int(call(u, "SendMessageW", zoomChoice, 0x147, 0, 0))
	if zi >= 0 && zi <= 3 {
		zoomIndex = zi
	}
	trace(fmt.Sprintf("preview start font=%d zoom=%d dpi=%d", sampleIndex, zoomIndex, dpi))
	w, h := int(sc(404)), int(sc(202))
	if w < 17 || h < 17 || w > 4096 || h > 4096 {
		return
	}
	factor := 1.0
	if zoomIndex == 0 {
		for _, im := range samples[sampleIndex] {
			if f := float64(w-16) / float64(im.Bounds().Dx()); f < factor {
				factor = f
			}
			if f := float64(h-16) / float64(im.Bounds().Dy()); f < factor {
				factor = f
			}
		}
	} else {
		factor = float64(int(1) << (zoomIndex - 1))
	}
	var next [2][]byte
	for side := 0; side < 2; side++ {
		j := side
		if mapping[sampleIndex] == 2 {
			j = 1 - side
		}
		im := samples[sampleIndex][j]
		sw, sh := im.Bounds().Dx(), im.Bounds().Dy()
		dw, dh := int(float64(sw)*factor), int(float64(sh)*factor)
		ox, oy := 8, 8
		if dw < w-16 {
			ox = (w - dw) / 2
		}
		if dh < h-16 {
			oy = (h - dh) / 2
		}
		buf := make([]byte, w*h*4)
		for q := range buf {
			buf[q] = 255
		}
		for y := 0; y < dh && y+oy < h; y++ {
			for x := 0; x < dw && x+ox < w; x++ {
				r, gg, bb, _ := im.At(int(float64(x)/factor), int(float64(y)/factor)).RGBA()
				v := ((y+oy)*w + x + ox) * 4
				buf[v] = byte(bb >> 8)
				buf[v+1] = byte(gg >> 8)
				buf[v+2] = byte(r >> 8)
				buf[v+3] = 0
			}
		}
		next[side] = buf
	}
	frames = next
	frameW = w
	frameH = h
	for side := 0; side < 2; side++ {
		label := []string{"BEFORE  |  原廠預設", "AFTER  |  FreeType 優化"}[side]
		call(u, "SetWindowTextW", labels[side], p(label))
		call(u, "InvalidateRect", previews[side], 0, 1)
	}
	txt := "真實 LINE 截圖・Windows 11 / 200% DPI・未經美化"
	if zoomIndex > 0 {
		txt += "｜局部原始像素"
	}
	call(u, "SetWindowTextW", note, p(txt))
	trace(fmt.Sprintf("preview complete font=%d zoom=%d", sampleIndex, zoomIndex))
}

// Only remove the known, private V1.6 runtime cache; never follow reparse points.
func cleanLegacyPreview() {
	local := os.Getenv("LOCALAPPDATA")
	if local == "" || !filepath.IsAbs(local) {
		return
	}
	for _, dir := range []string{base, filepath.Join(base, "preview-qt-6.9.0-x64")} {
		name, err := syscall.UTF16PtrFromString(dir)
		if err != nil {
			return
		}
		attr, err := syscall.GetFileAttributes(name)
		if err != nil || attr&0x400 != 0 || attr&0x10 == 0 {
			return
		}
	}
	path := filepath.Join(base, "preview-qt-6.9.0-x64")
	// Do not touch a cache while a legacy preview or installer is active.
	h, err := syscall.CreateToolhelp32Snapshot(2, 0)
	if err != nil {
		return
	}
	defer syscall.CloseHandle(h)
	var pe syscall.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err = syscall.Process32First(h, &pe); err == nil; err = syscall.Process32Next(h, &pe) {
		n := strings.ToLower(syscall.UTF16ToString(pe.ExeFile[:]))
		if n == "qml.exe" || strings.Contains(n, "line_font_enhancer_v1.6") {
			trace("legacy cache cleanup skipped: active process")
			return
		}
	}
	err = filepath.WalkDir(path, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		n, e := syscall.UTF16PtrFromString(p)
		if e != nil {
			return e
		}
		a, e := syscall.GetFileAttributes(n)
		if e != nil {
			return e
		}
		if a&0x400 != 0 {
			return errors.New("reparse point")
		}
		return nil
	})
	if err != nil {
		trace("legacy cache cleanup skipped: " + err.Error())
		return
	}
	if err = os.RemoveAll(path); err != nil {
		trace("legacy cache cleanup: " + err.Error())
	} else {
		trace("legacy cache removed")
	}
}
