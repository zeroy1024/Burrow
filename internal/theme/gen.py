import json, copy, re
from tokens_src import *

def get(tree,path):
    n=tree
    for k in path.split('.'): n=n[k]
    return n
def leaves(tree,prefix=''):
    for k,v in tree.items():
        p=prefix+k
        if isinstance(v,T) or not isinstance(v,dict): yield p,v
        else: yield from leaves(v,p+'.')
isref=lambda v:isinstance(v,str) and v.startswith('{') and v.endswith('}')

# ---- build dark semantic tree
def theme_tree(name):
    t=copy.deepcopy(SEM)
    if name=='dark':
        for p,v in DARK.items():
            leaf=get(t,p); leaf.value=v; leaf.derived=True
    return t
THEMES={'light':theme_tree('light'),'dark':theme_tree('dark')}

def rpal(v):
    # resolve palette refs recursively in value
    if isref(v):
        p=v[1:-1]; assert p.startswith('palette.'),v
        return rpal(get(PALETTE,p[8:]).value)
    if isinstance(v,list): return [rpal(x) for x in v]
    if isinstance(v,dict): return {k:rpal(x) for k,x in v.items()}
    return v
def sem_leaf(theme,path): return get(THEMES[theme],path)
def comp_leaves():
    for g,d in COMP.items():
        for k,ref in d.items(): yield g,k,ref

# validate comp refs
for g,k,r in comp_leaves():
    try: sem_leaf('light',r)
    except Exception as e: raise SystemExit(f'bad ref {g}.{k} -> {r}')

# ---------- value formatting
def parse_color(v):
    v=rpal(v)
    if v.startswith('#'): h=v[1:]; return int(h[0:2],16),int(h[2:4],16),int(h[4:6],16),1.0
    m=re.match(r'rgba\(([\d.]+),([\d.]+),([\d.]+),([\d.]+)\)',v); r,g,b,a=m.groups(); return int(r),int(g),int(b),float(a)
def css_color(v):
    r,g,b,a=parse_color(v); return f'rgba({r},{g},{b},{a:g})' if a<1 else '#%02x%02x%02x'%(r,g,b)
def json_val(t,v):
    if isref(v): return v
    if t=='dimension': return f'{v:g}px'
    if t=='duration': return f'{v:g}ms'
    if t=='shadow': return [{'offsetX':f"{l['x']:g}px",'offsetY':f"{l['y']:g}px",'blur':f"{l['blur']:g}px",'spread':f"{l['spread']:g}px",'color':l['color'],'inset':l['inset']} for l in v]
    if t=='spring': return {k:(f'{x:g}ms' if k=='duration' else x) for k,x in v.items()}
    return v
def jleaf(t,derived=None):
    o={'$type':t.type,'$value':json_val(t.type,t.value)}
    if t.desc: o['$description']=t.desc
    if derived or t.derived: o['$extensions']={'app.derived':True}
    return o
def jtree(tree):
    o={}
    for k,v in tree.items(): o[k]=jleaf(v) if isinstance(v,T) else jtree(v)
    return o
def nest(d,path,val):
    ks=path.split('.')
    for k in ks[:-1]: d=d.setdefault(k,{})
    d[ks[-1]]=val
dark_over={}
for p,v in DARK.items():
    lf=sem_leaf('dark',p); nest(dark_over,p,jleaf(lf,True))
comp_json={}
for g,k,r in comp_leaves():
    lf=sem_leaf('light',r); o={'$type':lf.type,'$value':'{'+r+'}'}
    if f'{g}.{k}' in DERIVED_COMP or g in DERIVED_COMP: o['$extensions']={'app.derived':True}; o['$description']=DERIVED_COMP.get(f'{g}.{k}',DERIVED_COMP.get(g))
    if f'{g}.{k}' in DEPRECATED_COMP: o['$description']='deprecated: '+DEPRECATED_COMP[f'{g}.{k}']
    nest(comp_json,f'{g}.{k}',o)
J={'$description':'SSH/SFTP/FTP 客户端 UI design tokens（不含终端内容配色）。三层：palette → theme.<name>（semantic）→ component。component 只引用 semantic 路径（不带主题前缀），由当前主题解析；semantic 只引用 {palette.*}。',
   '$extensions':{'app.referenceRules':{'palette':'raw values only','theme':'values or {palette.*}','component':'semantic paths only, e.g. {color.text.primary}'},'app.derivedFlag':'$extensions.app.derived = true 表示设计稿中没有、为补全体系推导的值'},
   'palette':jtree(PALETTE),
   'theme':{'light':{'$description':'当前设计稿（ssh-v6 / focus D / sider-drag / ftp）',**jtree(SEM)},
            'dark':{'$description':'推导示例（全部 derived），只覆盖 semantic 层，其余继承 light','$extends':'light',**dark_over}},
   'component':comp_json}
open('tokens.json','w').write(json.dumps(J,ensure_ascii=False,indent=2))

# ---------- counts
cnt=lambda tree:sum(1 for _ in leaves(tree))
COUNTS={'palette':cnt(PALETTE),'semantic':cnt(SEM),'darkOverrides':len(DARK),'component':sum(len(d) for d in COMP.values())}
der_pal=sum(1 for p,v in leaves(PALETTE) if v.derived)
der_sem=[p for p,v in leaves(SEM) if v.derived]
COUNTS.update(derivedPalette=der_pal,derivedSemanticLight=der_sem,derivedComponent=list(DERIVED_COMP))
json.dump(COUNTS,open('/tmp/counts.json','w'),ensure_ascii=False)

# ---------- Go
def camel(k):
    k=k.replace('-','_')
    if k[0].isdigit(): return 'N'+k
    return k[0].upper()+k[1:]
GOT={'color':'color.NRGBA','dimension':'float32','number':'float32','fontWeight':'int','fontFamily':'[]string','duration':'time.Duration','shadow':'Shadow','spring':'Spring','cubicBezier':'Bezier','string':'string'}
types=[]  # (name, fields[(fname,type,comment)])
def gen_types(tree,name,doc=''):
    fields=[]
    for k,v in tree.items():
        if isinstance(v,T): fields.append((camel(k),GOT[v.type],v.desc+(' [derived]' if v.derived else '')))
        else:
            sub=name+camel(k); gen_types(v,sub); fields.append((camel(k),sub,''))
    types.append((name,fields,doc))
def f32(x): return repr(float(x)).rstrip('0').rstrip('.') if float(x)!=int(float(x)) else str(int(float(x)))
def go_lit(t,v,ctx):
    # ctx: 'pal' literal only, 'sem' may ref palette
    if isref(v):
        p=v[1:-1]; assert p.startswith('palette.')
        return 'Palette.'+'.'.join(camel(x) for x in p[8:].split('.'))
    if t=='color':
        r,g,b,a=parse_color(v); return f'color.NRGBA{{R: 0x{r:02x}, G: 0x{g:02x}, B: 0x{b:02x}, A: 0x{round(a*255):02x}}}'
    if t in('dimension','number'): return f32(v)
    if t=='fontWeight': return str(v)
    if t=='fontFamily': return '[]string{'+', '.join(json.dumps(x) for x in v)+'}'
    if t=='duration':
        if v and float(v) != int(v): return f'{round(v * 1_000_000)} * time.Nanosecond'
        return f'{int(v)} * time.Millisecond' if v else '0'
    if t=='cubicBezier': return 'Bezier{'+', '.join(f32(x) for x in v)+'}'
    if t=='spring':
        parts=[]
        if 'stiffness' in v: parts+= [f"Stiffness: {f32(v['stiffness'])}",f"Damping: {f32(v['damping'])}"]
        if 'duration' in v: parts+= [f"Duration: {int(v['duration'])} * time.Millisecond",f"Bounce: {f32(v['bounce'])}"]
        return 'Spring{'+', '.join(parts)+'}'
    if t=='shadow':
        ls=[]
        for l in v:
            ls.append(f"{{X: {f32(l['x'])}, Y: {f32(l['y'])}, Blur: {f32(l['blur'])}, Spread: {f32(l['spread'])}, Color: {go_lit('color',l['color'],ctx)}"+(', Inset: true' if l['inset'] else '')+'}')
        return 'Shadow{'+', '.join(ls)+'}'
    return json.dumps(v)
def go_tree_lit(tree,name,ctx,ind=1):
    tab='\t'*ind; out=name+'{\n'
    for k,v in tree.items():
        if isinstance(v,T): out+=f'{tab}{camel(k)}: {go_lit(v.type,v.value,ctx)},\n'
        else: out+=f'{tab}{camel(k)}: '+go_tree_lit(v,name+camel(k),ctx,ind+1)+',\n'
    return out+'\t'*(ind-1)+'}'
gen_types(PALETTE,'PaletteTokens','原始色板与刻度（primitives）。主题不应直接修改。')
SEMNAMES={'color':'Colors','radius':'Radii','space':'Spacing','size':'Sizes','font':'Typography','shadow':'Shadows','glass':'Glass','opacity':'Opacities','motion':'Motion'}
for k,v in SEM.items(): gen_types(v,SEMNAMES[k])
semfields=[(camel(k),SEMNAMES[k],'') for k in SEM]
types.append(('Semantic',semfields,'语义层：主题只覆盖这一层。'))
compfields=[]
for g,d in COMP.items():
    tn=camel(g)+'Tokens'; fs=[]
    for k,r in d.items():
        lf=sem_leaf('light',r); comment='← '+r
        if f'{g}.{k}' in DEPRECATED_COMP: comment+='; Deprecated: '+DEPRECATED_COMP[f'{g}.{k}']
        fs.append((camel(k),GOT[lf.type],comment))
    types.append((tn,fs,'')); compfields.append((camel(g),tn,''))
types.append(('Components',compfields,'组件层：只由 Semantic 计算得出（见 buildComponents）。'))
def semexpr(path):
    ks=path.split('.'); return 's.'+'.'.join(camel(k) for k in ks)
def semname(path):  # type name prefix for nested literal
    pass
go=['// Code generated by gen.py from tokens_src.py; DO NOT EDIT by hand — edit tokens_src.py and re-run.','',
'// Package theme holds the design tokens of the SSH/SFTP/FTP client UI',
'// (everything except terminal content rendering). Three tiers:',
'//',
'//\tPalette (primitives) -> Semantic (per theme) -> Components (computed from Semantic)',
'//',
'// Themes override only the Semantic tier; components are rebuilt from it, so',
'// swapping themes never touches component code. The package is framework-agnostic:',
'// colors are image/color.NRGBA, sizes are float32 logical px (dp), durations are',
'// time.Duration and springs carry stiffness/damping or duration/bounce. See',
'// adapter notes in tokens.md for mapping to MyGo (package ui).',
'package theme','','import (','\t"image/color"','\t"math"','\t"sync"','\t"time"',')','',
'''// ShadowLayer is one box-shadow layer. Spread>0 with Blur=0 draws a ring (outline).
type ShadowLayer struct {
	X, Y, Blur, Spread float32
	Color              color.NRGBA
	Inset              bool
}

// Shadow is an ordered list of layers (first drawn on top, like CSS).
type Shadow []ShadowLayer

// Spring describes a spring animation. Either Stiffness/Damping (physical) or
// Duration/Bounce (perceptual, like SwiftUI / MyGo SpringOptions) is set.
type Spring struct {
	Stiffness float64
	Damping   float64
	Duration  time.Duration
	Bounce    float64
}

// Bezier is a cubic-bezier easing curve (x1, y1, x2, y2).
type Bezier [4]float32
''']
for name,fields,doc in types:
    if doc: go.append(f'// {name} {doc}')
    else: go.append(f'// {name} groups tokens.')
    go.append(f'type {name} struct {{')
    for fn,ft,c in fields: go.append(f'\t{fn} {ft}'+(f' // {c}' if c else ''))
    go.append('}\n')
go.append('// Palette is the primitive tier.\nvar Palette = '+go_tree_lit(PALETTE,'PaletteTokens','pal')+'\n')
go.append('func lightSemantic() Semantic {\n\treturn Semantic{')
for k,v in SEM.items(): go.append(f'\t\t{camel(k)}: '+go_tree_lit(v,SEMNAMES[k],'sem',3)+',')
go.append('\t}\n}\n')
go.append('// darkOverrides applies the derived dark theme on top of light (semantic tier only).\nfunc darkOverrides(s *Semantic) {')
for p,v in DARK.items():
    lf=sem_leaf('light',p); go.append(f'\t{semexpr(p)} = {go_lit(lf.type,v,"sem")}')
go.append('}\n')
go.append('func buildComponents(s *Semantic) Components {\n\treturn Components{')
for g,d in COMP.items():
    go.append(f'\t\t{camel(g)}: {camel(g)}Tokens{{')
    for k,r in d.items(): go.append(f'\t\t\t{camel(k)}: {semexpr(r)},')
    go.append('\t\t},')
go.append('\t}\n}\n')
go.append('''// Theme is a resolved theme: semantic tokens plus components built from them.
type Theme struct {
	Name   string
	IsDark bool
	Semantic
	C Components
}

// New builds a theme from a semantic tier (use it for custom themes:
// start from LightSemantic(), override fields, call New).
func New(name string, dark bool, s Semantic) *Theme {
	return &Theme{Name: name, IsDark: dark, Semantic: s, C: buildComponents(&s)}
}

// LightSemantic returns a fresh copy of the light semantic tier.
func LightSemantic() Semantic { return lightSemantic() }

// DarkSemantic returns a fresh copy of the (derived) dark semantic tier.
func DarkSemantic() Semantic { s := lightSemantic(); darkOverrides(&s); return s }

// Light is the current design (ssh-v6 / focus D / sider-drag / ftp mockups).
func Light() *Theme { return New("light", false, LightSemantic()) }

// Dark is a derived example proving theme swapping.
func Dark() *Theme { return New("dark", true, DarkSemantic()) }

// ForAppearance returns Dark() when dark is true, otherwise Light().
func ForAppearance(dark bool) *Theme {
	if dark {
		return Dark()
	}
	return Light()
}

var (
	mu        sync.RWMutex
	current   = Light()
	listeners = map[int]func(*Theme){}
	nextID    int
)

// Current returns the active theme. Safe for concurrent use; treat the result as read-only.
func Current() *Theme {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// Set replaces the active theme and notifies listeners (call from the UI
// thread, then request a redraw / invalidate so the GPU UI re-reads tokens).
func Set(t *Theme) {
	if t == nil {
		return
	}
	mu.Lock()
	current = t
	ls := make([]func(*Theme), 0, len(listeners))
	for _, f := range listeners {
		ls = append(ls, f)
	}
	mu.Unlock()
	for _, f := range ls {
		f(t)
	}
}

// OnChange registers a listener and returns a function that removes it.
func OnChange(f func(*Theme)) (cancel func()) {
	mu.Lock()
	defer mu.Unlock()
	id := nextID
	nextID++
	listeners[id] = f
	return func() {
		mu.Lock()
		delete(listeners, id)
		mu.Unlock()
	}
}

// WithAlpha returns c with alpha a (0..1).
func WithAlpha(c color.NRGBA, a float64) color.NRGBA {
	c.A = uint8(math.Round(clamp01(a) * 255))
	return c
}

// Mix blends a toward b by t (0 = a, 1 = b), in sRGB like CSS color-mix(in srgb).
func Mix(a, b color.NRGBA, t float64) color.NRGBA {
	t = clamp01(t)
	l := func(x, y uint8) uint8 { return uint8(math.Round(float64(x)*(1-t) + float64(y)*t)) }
	return color.NRGBA{R: l(a.R, b.R), G: l(a.G, b.G), B: l(a.B, b.B), A: l(a.A, b.A)}
}

// OSLogoBg returns the tinted square behind an OS logo: brand color mixed into
// the card surface (color-mix(in srgb, brand 9%, card)).
func (t *Theme) OSLogoBg(brand color.NRGBA) color.NRGBA {
	return Mix(t.C.OsLogo.BgBase, brand, float64(t.C.OsLogo.TintMix))
}

// MotionDuration returns d, or 0 when reduce-motion is on (positional animation becomes
// instant; keep color/opacity fades as the spec says).
func (t *Theme) MotionDuration(d time.Duration, reduceMotion bool) time.Duration {
	if reduceMotion {
		return 0
	}
	return d
}

// Hex parses "#rrggbb" or "#rrggbbaa" (for user/custom themes). Invalid input returns opaque black.
func Hex(s string) color.NRGBA {
	if len(s) > 0 && s[0] == '#' {
		s = s[1:]
	}
	if len(s) != 6 && len(s) != 8 {
		return color.NRGBA{A: 0xff}
	}
	var v [4]uint8
	v[3] = 0xff
	for i := 0; i < len(s)/2; i++ {
		v[i] = hexByte(s[2*i])<<4 | hexByte(s[2*i+1])
	}
	return color.NRGBA{R: v[0], G: v[1], B: v[2], A: v[3]}
}

func hexByte(c byte) uint8 {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

func clamp01(x float64) float64 { return math.Max(0, math.Min(1, x)) }
''')
open('theme.go','w').write('\n'.join(go))

# ---------- resolved values for md/preview
def resolve_sem(theme,path):
    lf=sem_leaf(theme,path); return lf.type,rpal(lf.value)
def fmt(t,v):
    if t=='color': return css_color(v)
    if t=='dimension': return f'{v:g}px'
    if t=='duration': return f'{v:g}ms'
    if t=='number': return f'{v:g}'
    if t=='fontFamily': return ', '.join(v)
    if t=='shadow': return ', '.join(('inset ' if l['inset'] else '')+f"{l['x']:g}px {l['y']:g}px {l['blur']:g}px {l['spread']:g}px {css_color(l['color'])}" for l in v)
    if t=='spring': return ' · '.join(f'{k} {x:g}'+('ms' if k=='duration' else '') for k,x in v.items())
    if t=='cubicBezier': return 'cubic-bezier('+','.join(f'{x:g}' for x in v)+')'
    return str(v)
import pickle
RES={th:{p:fmt(*resolve_sem(th,p)) for p,_ in leaves(SEM)} for th in THEMES}
TYP={p:v.type for p,v in leaves(SEM)}
pickle.dump(dict(RES=RES,TYP=TYP),open('/tmp/res.pkl','wb'))
print(COUNTS)
