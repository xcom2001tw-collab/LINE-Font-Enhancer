from PIL import Image,ImageDraw,ImageFont
import struct,io
S=256
im=Image.new('RGBA',(S,S)); d=ImageDraw.Draw(im)
d.rounded_rectangle((3,3,253,253),radius=58,fill='#08934f')
d.rounded_rectangle((36,44,220,177),radius=30,fill='white');d.polygon([(71,170),(64,212),(114,174)],fill='white')
f=ImageFont.truetype('/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf',84)
d.text((128,108),'Aa',font=f,anchor='mm',fill='#08934f');d.polygon([(220,12),(229,34),(251,43),(229,52),(220,74),(211,52),(189,43),(211,34)],fill='#ffda75')
sizes=[16,32,48,64,128,256]; imgs=[]
for s in sizes:
 b=io.BytesIO();im.resize((s,s),Image.Resampling.LANCZOS).save(b,format='PNG');imgs.append(b.getvalue())
group=struct.pack('<HHH',0,1,len(sizes))+b''.join(struct.pack('<BBBBHHIH',s%256,s%256,0,0,1,32,len(b),i+1) for i,(s,b) in enumerate(zip(sizes,imgs)))
# COFF resource section: resource directory tree, data entries, raw payloads.
buf=bytearray(); rel=[]
def reserve(n):
 o=len(buf);buf.extend(b'\0'*n);return o
def write(o,fmt,*v):struct.pack_into(fmt,buf,o,*v)
def tree(items):
 o=reserve(16+8*len(items));write(o+12,'<HH',0,len(items))
 for i,(key,val) in enumerate(items):
  if isinstance(val,list):target=tree(val)|0x80000000
  else:
   target=reserve(16); payload=reserve(len(val));buf[payload:payload+len(val)]=val
   write(target,'<IIII',payload,len(val),0,0);rel.append(target)
   reserve((-len(buf))%4)
  write(o+16+i*8,'<II',key,target)
 return o
tree([(3,[(i+1,[(1033,b)]) for i,b in enumerate(imgs)]),(14,[(1,[(1033,group)])])])
raw=60; relo=raw+len(buf); sym=relo+10*len(rel)
header=struct.pack('<HHIIIHH',0x8664,1,0,sym,1,0,0)
section=struct.pack('<8sIIIIIIHHI',b'.rsrc\0\0\0',0,0,len(buf),raw,relo,0,len(rel),0,0x40300040)
reloc=b''.join(struct.pack('<IIH',o,0,3) for o in rel)
symbol=struct.pack('<8sIhHBB',b'.rsrc\0\0\0',0,1,0,3,0)
open('icon_windows_amd64.syso','wb').write(header+section+buf+reloc+symbol+struct.pack('<I',4))
im.save('icon.png')
