# Single source of truth. Generates tokens.json, theme.go, tokens.md tables, preview css.
# Leaf = T(type, value, desc, derived). Values: literal or "{ref.path}".
class T:
    def __init__(s,type,value,desc='',derived=False):
        s.type,s.value,s.desc,s.derived=type,value,desc,derived
C=lambda v,d='',x=False:T('color',v,d,x)
D=lambda v,d='',x=False:T('dimension',v,d,x)
N=lambda v,d='',x=False:T('number',v,d,x)
W=lambda v,d='',x=False:T('fontWeight',v,d,x)
F=lambda v,d='',x=False:T('fontFamily',v,d,x)
MS=lambda v,d='',x=False:T('duration',v,d,x)
SH=lambda v,d='',x=False:T('shadow',v,d,x)
SP=lambda v,d='',x=False:T('spring',v,d,x)
BZ=lambda v,d='',x=False:T('cubicBezier',v,d,x)
def L(x,y,b,s,c,inset=False): return dict(x=x,y=y,blur=b,spread=s,color=c,inset=inset)

# ------------------------------------------------------------------ 1. PRIMITIVES (palette)
A=lambda base,a: f'rgba({base},{a})'
PALETTE={
 'gray':{'0':C('#ffffff'),'25':C('#fbfbfc'),'40':C('#fafafb'),'50':C('#f7f7f8'),'60':C('#f6f6f7'),'75':C('#f4f4f5'),
   '100':C('#f2f2f4'),'125':C('#f0f0f2'),'150':C('#efeff1'),'175':C('#ececef'),'190':C('#ededf0'),'200':C('#e7e7e8'),'225':C('#e6e6e9'),
   '250':C('#e3e3e6'),'300':C('#d6d6d8'),'325':C('#d2d2d6'),'350':C('#cfcfd4'),'400':C('#c7c7cc'),'450':C('#b0b0b5'),
   '475':C('#aeaeb2'),'500':C('#a1a1a6'),'600':C('#8e8e93'),'700':C('#6e6e73'),'750':C('#5b5b62'),'800':C('#4a4a50'),
   '850':C('#3a3a3f'),'900':C('#1d1d1f')},
 'blue':{'50':C('#eaf3fe'),'75':C('#e8f2fe'),'100':C('#d7e9fd','文件夹图标填充'),'300':C('#5ba7f7'),'500':C('#0a84ff','品牌蓝 / accent（logo 同色）'),'600':C('#0a6fd6'),'700':C('#0a5bc4')},
 'green':{'50':C('#ebf8ef'),'500':C('#30c45a'),'700':C('#1f8a3f')},
 'amber':{'50':C('#fdf3e3'),'500':C('#f5a623'),'600':C('#c7861a'),'700':C('#b26a00')},
 'traffic':{'red':C('#ff5f57'),'yellow':C('#febc2e'),'green':C('#28c840')},
 'black':{k:C(A('0,0,0',v)) for k,v in [('a035','.035'),('a04','.04'),('a05','.05'),('a06','.06'),('a07','.07'),('a08','.08'),('a09','.09'),('a10','.10'),('a12','.12'),('a14','.14'),('a15','.15'),('a16','.16'),('a18','.18')]},
 'white':{k:C(A('255,255,255',v)) for k,v in [('a30','.30'),('a42','.42'),('a60','.60'),('a70','.70'),('a85','.85'),('a90','.90')]},
 'blueA':{k:C(A('10,132,255',v)) for k,v in [('a035','.035'),('a06','.06'),('a08','.08'),('a10','.10'),('a13','.13'),('a15','.15'),('a16','.16'),('a18','.18'),('a28','.28'),('a35','.35'),('a50','.50')]},
 'greenA':{'a14':C(A('48,196,90','.14')),'a15':C(A('48,196,90','.15'))},
 'amberA':{'a14':C(A('245,166,35','.14')),'a18':C(A('245,166,35','.18'))},
 'ink':{'a92':C(A('30,30,32','.92'),'tooltip 底')},
 # dark-only primitives (all derived)
 'graphite':{k:C(v,'深色主题',True) for k,v in [('950','#161618'),('900','#1c1c1e'),('850','#202022'),('800','#252527'),('750','#2a2a2d'),('700','#2e2e31'),('650','#333336'),('600','#3a3a3d'),('550','#444448'),('500','#5a5a5f'),('400','#6e6e73'),('300','#8e8e93'),('250','#98989d'),('200','#a1a1a6'),('150','#c7c7cc'),('100','#d1d1d6'),('50','#f5f5f7')]},
 'darkTint':{k:C(v,'深色主题',True) for k,v in [('blue300','#4da3ff'),('blue200','#6cb4ff'),('blueSubtle','#16304d'),('green400','#32d74b'),('green300','#5fd97a'),('greenSubtle','#173523'),('amber400','#ffb340'),('amber300','#ffc15c'),('amberSubtle','#3d2e12')]},
 'whiteA':{k:C(A('255,255,255',v),'深色主题',True) for k,v in [('a06','.06'),('a08','.08'),('a10','.10'),('a12','.12'),('a14','.14'),('a16','.16'),('a20','.20')]},
 'blackA':{k:C(A('0,0,0',v),'深色主题',True) for k,v in [('a25','.25'),('a30','.30'),('a35','.35'),('a40','.40'),('a45','.45'),('a50','.50')]},
 'inkDark':{'a95':C(A('58,58,62','.95'),'深色 tooltip 底',True)},
 'space':{str(v):D(v) for v in [0,1,2,3,4,5,6,7,8,9,10,12,14,15,16,18,22,24]},
 'radius':{str(v):D(v) for v in [1,2,3,4,5,6,7,8,9,10,12,20]},
 'size':{str(v).replace('.','_'):D(v) for v in [0.5,1,1.5,1.75,2,3,4,5,6,7,12,14,15,16,18,22,24,26,28,30,32,34,36,40,44,48,52,56,204,236,296]},
 'fontSize':{k:D(v) for k,v in [('10_5',10.5),('11',11),('11_5',11.5),('12',12),('12_5',12.5),('13',13),('15',15)]},
 'lineHeight':{k:D(v) for k,v in [('14',14),('15',15),('16',16),('17',17)]},
 'fontWeight':{'400':W(400),'500':W(500),'600':W(600)},
 'fontFamily':{'system':F(['-apple-system','SF Pro Text','PingFang SC','Inter','Noto Sans CJK SC','system-ui','sans-serif']),
   'mono':F(['JetBrains Mono','Menlo','monospace'],'仅用于主机名/路径等 UI 文本，不用于终端内容')},
 'letterSpacing':{'neg0_2':D(-0.2),'zero':D(0),'pos0_3':D(0.3)},
 'blur':{str(v).replace('.','_'):D(v) for v in [1,1.5,3,4,5,6,9,16]},
 'ratio':{k:N(v) for k,v in [('0',0),('0_09',.09),('0_15',.15),('0_18',.18),('0_3',.3),('0_35',.35),('0_4',.4),('0_45',.45),('0_5',.5),('0_55',.55),('0_6',.6),('0_7',.7),('0_78',.78),('0_8',.8),('0_92',.92),('1',1),('1_03',1.03),('1_5',1.5),('1_8',1.8)]},
 'duration':{str(v):MS(v) for v in [0,120,150,160,180,220,300,320,500,600]},
 'speed':{'autoScroll':N(900,'px/s')},
 'spring':{'snappy':SP({'stiffness':420,'damping':34}),'drop':SP({'duration':320,'bounce':.15}),'smooth':SP({'duration':300,'bounce':0})},
 'easing':{'easeOut':BZ([0,0,.58,1]),'standard':BZ([.25,.1,.25,1],'CSS ease',True)},
}
P=lambda path:'{palette.'+path+'}'

# ------------------------------------------------------------------ 2. SEMANTIC (light = current design)
SEM={
 'color':{
  'bg':{'window':C(P('gray.200'),'窗口底色（标题栏、rail、卡片间缝隙）')},
  'surface':{
    'card':C(P('gray.0'),'内容卡片 / sider / FTP 面板'),
    'terminal':C(P('gray.25'),'终端卡片容器底色（仅容器，不含终端文字配色）'),
    'raised':C(P('gray.0'),'浮起层：拖动卡片、ghost、分段控件选中'),
    'sunken':C(P('gray.75'),'搜索框'),
    'field':C(P('gray.60'),'面包屑路径框'),
    'zebra':C(P('gray.40'),'列表斑马纹'),
    'control':C(P('gray.100'),'中性控件底：图标按钮 bg、分段控件、中性徽章、队列图标'),
    'controlStrong':C(P('gray.175'),'sider 行内「更多」按钮'),
    'keycap':C(P('gray.0'),'键帽 kbd 底'),
  },
  'state':{
    'hover':C(P('gray.50'),'行悬停'),
    'selected':C(P('gray.150'),'sider 选中行'),
    'pressed':C(P('gray.225'),'按下（设计稿未画）',True),
    'onWindow':C(P('gray.300'),'窗口底色上的激活按钮（rail、标题栏开关）'),
    'selectedAccent':C(P('blue.75'),'文件列表选中行'),
  },
  'border':{
    'card':C(P('black.a06'),'卡片 0.5px 外描边'),
    'terminal':C(P('black.a08'),'终端/窗格卡片 0.5px 内描边'),
    'subtle':C(P('gray.125'),'卡片内分隔线（sider footer、状态栏、进度轨道）'),
    'divider':C(P('gray.100'),'头部底线、表头底线'),
    'tab':C(P('gray.150'),'旧版 tab 线 / FTP 面板描边'),
    'control':C(P('gray.225'),'按钮描边、状态栏竖分隔'),
    'keycap':C(P('gray.250'),'kbd 描边'),
    'separatorWindow':C(P('gray.325'),'标题栏按钮组分隔线'),
    'onGlass':C(P('black.a10'),'玻璃栏内竖分隔'),
    'raised':C(P('black.a08'),'浮起层 0.5px 描边'),
    'hairlineStrong':C(P('black.a10'),'拖动卡片描边'),
  },
  'text':{
    'primary':C(P('gray.900')),
    'secondary':C(P('gray.600'),'次要文字、主机副标题、状态栏'),
    'tertiary':C(P('gray.500'),'关闭 ×、kbd、caption'),
    'quaternary':C(P('gray.450'),'分组计数'),
    'muted':C(P('gray.700'),'未选中 tab、rail 图标、按钮图标'),
    'strong':C(P('gray.850'),'次级按钮文字'),
    'action':C(P('gray.800'),'标题栏动作按钮'),
    'neutralBadge':C(P('gray.750'),'中性徽章文字'),
    'disabled':C(P('gray.475'),'非焦点窗格主机'),
    'placeholder':C(P('gray.400'),'面包屑分隔符'),
    'onAccent':C(P('gray.0')),
    'accent':C(P('blue.600'),'蓝色文字（徽章、提示、高亮分组）'),
    'accentStrong':C(P('blue.700'),'选中文件名'),
  },
  'icon':{'default':C(P('gray.700')),'subtle':C(P('gray.600')),'faint':C(P('gray.500')),'folder':C(P('blue.300')),'folderFill':C(P('blue.100'))},
  'accent':{
    'default':C(P('blue.500'),'主强调色 #0a84ff'),
    'subtle':C(P('blue.50'),'浅蓝底（上传、运行中、提示）'),
    'tint035':C(P('blueA.a035'),'落位区底'),
    'tint06':C(P('blueA.a06'),'可落位占位底'),
    'tint08':C(P('blueA.a08'),'分组拖入高亮'),
    'tint10':C(P('blueA.a10'),'新输出徽章 / 闪光'),
    'tint13':C(P('blueA.a13'),'自动滚动区渐变'),
    'tint15':C(P('blueA.a15'),'进度条轨道（自动展开）'),
    'tint16':C(P('blueA.a16'),'焦点光晕'),
    'tint18':C(P('blueA.a18'),'闪光内描边'),
    'tint28':C(P('blueA.a28'),'占位内描边'),
    'tint35':C(P('blueA.a35'),'标注描边'),
    'tint50':C(P('blueA.a50'),'自动滚动区边线'),
  },
  'success':{'default':C(P('green.500'),'在线点、完成进度'),'text':C(P('green.700')),'subtle':C(P('green.50')),'halo':C(P('greenA.a14'),'在线点 3px 光圈'),'onGlass':C(P('greenA.a15'))},
  'warning':{'default':C(P('amber.500'),'连接中'),'text':C(P('amber.600'),'连接中文字 / 响铃'),'textStrong':C(P('amber.700')),'subtle':C(P('amber.50')),'tint14':C(P('amberA.a14'),'响铃徽章底'),'halo':C(P('amberA.a18'))},
  'status':{'offline':C(P('gray.350'),'离线点'),'paused':C(P('gray.400'),'暂停进度')},
  'fill':{'quiet':C(P('black.a05'),'⌘N 芯片、玻璃上徽章'),'quieter':C(P('black.a04'),'玻璃上的面包屑'),'placeholder':C(P('black.a035'),'拖动占位'),'placeholderEdge':C(P('black.a05')),'strong':C(P('gray.900'),'中性进度条填充')},
  'traffic':{'close':C(P('traffic.red')),'minimize':C(P('traffic.yellow')),'zoom':C(P('traffic.green')),'edge':C(P('black.a15'))},
  'tooltip':{'bg':C(P('ink.a92')),'text':C(P('gray.0')),'kbd':C(P('white.a60'))},
  'glass':{'titleTab':C(P('white.a85'),'标题栏选中 tab'),'float':C(P('white.a70'),'悬浮胶囊（回到底部）'),'bar':C(P('white.a42'),'v4 玻璃栏（无渐隐时）'),'highlight':C(P('white.a90'),'玻璃顶部内高光')},
 },
 'radius':{'window':D(P('radius.12')),'card':D(P('radius.10')),'pane':D(P('radius.10')),'lift':D(P('radius.9')),'railButton':D(P('radius.8')),
   'dropZone':D(P('radius.8')),'control':D(P('radius.7')),'controlSm':D(P('radius.6')),'chip':D(P('radius.5')),'keycap':D(P('radius.4')),
   'bar':D(P('radius.3')),'pill':D(P('radius.20')),'badge':D(P('radius.9')),'hair':D(P('radius.2')),'tabTop':D(P('radius.8'))},
 'space':{k:D(P('space.'+k)) for k in ['0','1','2','3','4','5','6','7','8','9','10','12','14','15','16','18','22','24']},
 'size':{
   'hairline':D(P('size.0_5'),'0.5px 描边'),'divider':D(P('size.1')),'focusRing':D(P('size.1_5'),'焦点环宽 / 虚线落位框宽'),'focusHalo':D(P('size.5'),'焦点光晕扩散'),
   'dot':D(P('size.7')),'dotSm':D(P('size.6')),'dotHalo':D(P('size.3')),'trafficLight':D(P('size.12')),
   'icon':D(P('size.16')),'iconMd':D(P('size.15')),'iconSm':D(P('size.14')),'iconStroke':D(P('size.1_75'),'Lucide 风格线宽'),
   'controlXs':D(P('size.22')),'controlSm':D(P('size.24')),'controlMd':D(P('size.26')),'control':D(P('size.28')),'controlLg':D(P('size.30')),'railButton':D(P('size.34')),
   'titlebar':D(P('size.44')),'rail':D(P('size.52')),'sider':D(P('size.236')),'titleLeft':D(P('size.296'),'标题栏左区（交通灯 + 侧栏开关），与 rail+sider 对齐'),
   'paneHeader':D(P('size.36')),'statusBar':D(P('size.34')),'paneStatusBar':D(P('size.32')),'listRow':D(P('size.30')),'fileHeader':D(P('size.40')),'queueRow':D(P('size.56')),
   'groupRow':D(P('size.30')),'osLogo':D(P('size.26')),'osGlyph':D(P('size.15')),'progress':D(P('size.5')),'progressThin':D(P('size.2')),
   'dropLine':D(P('size.2'),'插入线（ListState.Reorder）',True),'fadeExtent':D(P('size.24'),'玻璃栏向内容延伸的渐隐长度'),'fadeExtentPane':D(P('size.22'),'分屏窗格内的渐隐长度'),
   'statsPanel':D(P('size.204'),'监控侧栏（分屏时）'),'autoScrollZone':D(P('size.48')),'separatorShort':D(P('size.12')),'separatorTall':D(P('size.16')),'separatorStatus':D(P('size.18')),
 },
 'font':{
   'family':{'ui':F(P('fontFamily.system')),'mono':F(P('fontFamily.mono'))},
   'size':{'caption2':D(P('fontSize.10_5'),'⌘N 芯片、计数胶囊'),'caption':D(P('fontSize.11'),'kbd、状态栏徽章、计数'),'footnote':D(P('fontSize.11_5'),'分组头、副标题、状态栏'),
           'small':D(P('fontSize.12'),'sider footer、主机副信息'),'body':D(P('fontSize.12_5'),'tab、按钮、列表'),'bodyLg':D(P('fontSize.13'),'主机名、标题'),'title':D(P('fontSize.15'),'大标题（sider-drag 版 sider 标题）')},
   'weight':{'regular':W(P('fontWeight.400')),'medium':W(P('fontWeight.500')),'semibold':W(P('fontWeight.600'))},
   'lineHeight':{'tight':D(P('lineHeight.14')),'sub':D(P('lineHeight.15')),'body':D(P('lineHeight.16')),'name':D(P('lineHeight.17'))},
   'tracking':{'mono':D(P('letterSpacing.neg0_2')),'normal':D(P('letterSpacing.zero')),'caps':D(P('letterSpacing.pos0_3'))},
 },
 'shadow':{
   'window':SH([L(0,0,0,.5,P('black.a18')),L(0,24,60,0,P('black.a18')),L(0,4,14,0,P('black.a08'))]),
   'card':SH([L(0,0,0,.5,P('black.a06'))],'卡片外描边'),
   'paneOutline':SH([L(0,0,0,.5,P('black.a08'),True)],'窗格内描边（clip 后叠加）'),
   'titleTab':SH([L(0,0,0,.5,P('black.a07')),L(0,1,2,0,P('black.a05'))]),
   'segment':SH([L(0,1,2,0,P('black.a08'))]),
   'float':SH([L(0,0,0,.5,P('black.a10')),L(0,4,12,0,P('black.a08'))],'悬浮胶囊'),
   'tooltip':SH([L(0,4,12,0,P('black.a18'))]),
   'ghost':SH([L(0,8,24,0,P('black.a12')),L(0,0,0,.5,P('black.a08'))],'FTP 拖拽 ghost'),
   'floatButton':SH([L(0,2,8,0,P('black.a10')),L(0,0,0,.5,P('black.a08'))],'FTP 中缝传输按钮'),
   'lift':SH([L(0,0,0,.5,P('black.a10')),L(0,10,28,0,P('black.a16')),L(0,2,6,0,P('black.a08'))],'拖动中卡片'),
   'liftStack':SH([L(0,0,0,.5,P('black.a08')),L(0,4,10,0,P('black.a06'))],'多选叠卡'),
   'focusRing':SH([L(0,0,0,1.5,P('blue.500'))],'焦点窗格：仅 1.5px 蓝色描边'),
   'hoverRing':SH([L(0,0,0,1,P('black.a14'))],'非焦点窗格悬停'),
   'flash':SH([L(0,0,0,1,P('blueA.a18'),True)],'落位闪光内描边'),
   'placeholder':SH([L(0,0,0,1,P('black.a05'),True)]),
   'placeholderAccent':SH([L(0,0,0,1,P('blueA.a28'),True)]),
   'osSelected':SH([L(0,0,0,.5,P('black.a08'))]),
   'glassHighlight':SH([L(0,.5,0,0,P('white.a90'),True)]),
   'dotOnline':SH([L(0,0,0,3,P('greenA.a14'))]),'dotConnecting':SH([L(0,0,0,3,P('amberA.a18'))]),
 },
 'glass':{
   'fade':{'blur1':D(P('blur.1')),'blur2':D(P('blur.3')),'blur3':D(P('blur.5')),
     'mask1Solid':N(P('ratio.0_45'),'层 1 实心到 45%，100% 透明'),'mask1End':N(P('ratio.1')),
     'mask2Solid':N(P('ratio.0_4')),'mask2End':N(P('ratio.0_78')),'mask3Solid':N(P('ratio.0_3')),'mask3End':N(P('ratio.0_6')),
     'tintStart':N(P('ratio.0_92'),'着色渐变起点 alpha（取所在卡片底色）'),'tintMid':N(P('ratio.0_78')),'tintMidStop':N(P('ratio.0_45')),'tintEnd':N(P('ratio.0'))},
   'float':{'blur':D(P('blur.16')),'saturate':N(P('ratio.1_5'))},
   'bar':{'blur':D(P('blur.3')),'saturate':N(P('ratio.1_8'))},
 },
 'opacity':{
   'windowTint':N(P('ratio.0_18'),'macOS 原生窗口材质上的统一底色着色强度',True),
   'titlebarTint':N(P('ratio.0_92'),'其他平台原生标题栏材质上的窗口底色着色强度',True),
   'paneUnfocusedContent':N(P('ratio.0_5')),'paneUnfocusedSaturate':N(P('ratio.0_35')),'paneUnfocusedStatus':N(P('ratio.0_55')),
   'paneHoverContent':N(P('ratio.0_78')),'paneHoverSaturate':N(P('ratio.0_7')),'dragSource':N(P('ratio.0_4'),'拖走后源行（折叠时）'),'liftStackBack':N(P('ratio.0_8')),
 },
 'motion':{
   'duration':{'instant':MS(P('duration.0')),'lift':MS(P('duration.120'),'拿起'),'hold':MS(P('duration.150'),'长按拿起'),'focus':MS(P('duration.160'),'焦点/悬停切换'),
     'collapse':MS(P('duration.180'),'分组拖动时全部收起'),'restore':MS(P('duration.220'),'恢复展开'),'cancel':MS(P('duration.300'),'取消飞回'),
     'drop':MS(P('duration.320'),'落位'),'autoExpand':MS(P('duration.500'),'悬停自动展开'),'flash':MS(P('duration.600'),'落位闪光')},
   'easing':{'out':BZ(P('easing.easeOut'))},
   'spring':{'displace':SP(P('spring.snappy'),'让位（≈ Snappy, 220ms）'),'drop':SP(P('spring.drop'),'落位'),'smooth':SP(P('spring.smooth'),'取消 / 平滑（ui.Smooth）')},
   'drag':{'threshold':D(P('size.4'),'移动阈值 px'),'liftScale':N(P('ratio.1_03')),'autoScrollMax':N(P('speed.autoScroll'),'px/s，t² 曲线'),'autoScrollZone':D(P('size.48'))},
 },
}

# ------------------------------------------------------------------ 2b. DARK overrides (all derived)
DARK={
 'color.bg.window':P('graphite.900'),
 'color.surface.card':P('graphite.800'),'color.surface.terminal':P('graphite.850'),'color.surface.raised':P('graphite.700'),
 'color.surface.sunken':P('graphite.750'),'color.surface.field':P('graphite.750'),'color.surface.zebra':P('whiteA.a06'),'color.surface.control':P('graphite.700'),
 'color.surface.controlStrong':P('graphite.650'),'color.surface.keycap':P('graphite.700'),
 'color.state.hover':P('graphite.750'),'color.state.selected':P('graphite.650'),'color.state.pressed':P('graphite.600'),'color.state.onWindow':P('graphite.600'),'color.state.selectedAccent':P('darkTint.blueSubtle'),
 'color.border.card':P('whiteA.a08'),'color.border.terminal':P('whiteA.a10'),'color.border.subtle':P('graphite.650'),'color.border.divider':P('graphite.650'),
 'color.border.tab':P('graphite.650'),'color.border.control':P('graphite.550'),'color.border.keycap':P('graphite.550'),'color.border.separatorWindow':P('graphite.550'),
 'color.border.onGlass':P('whiteA.a12'),'color.border.raised':P('whiteA.a10'),'color.border.hairlineStrong':P('whiteA.a12'),
 'color.text.primary':P('graphite.50'),'color.text.secondary':P('graphite.250'),'color.text.tertiary':P('graphite.400'),'color.text.quaternary':P('graphite.500'),
 'color.text.muted':P('graphite.200'),'color.text.strong':P('graphite.100'),'color.text.action':P('graphite.150'),'color.text.neutralBadge':P('graphite.150'),
 'color.text.disabled':P('graphite.400'),'color.text.placeholder':P('graphite.500'),'color.text.accent':P('darkTint.blue300'),'color.text.accentStrong':P('darkTint.blue200'),
 'color.icon.default':P('graphite.200'),'color.icon.subtle':P('graphite.300'),'color.icon.faint':P('graphite.400'),'color.icon.folderFill':P('darkTint.blueSubtle'),
 'color.accent.subtle':P('darkTint.blueSubtle'),'color.accent.tint16':P('blueA.a28'),'color.accent.tint10':P('blueA.a18'),
 'color.success.default':P('darkTint.green400'),'color.success.text':P('darkTint.green300'),'color.success.subtle':P('darkTint.greenSubtle'),
 'color.warning.default':P('darkTint.amber400'),'color.warning.text':P('darkTint.amber300'),'color.warning.textStrong':P('darkTint.amber300'),'color.warning.subtle':P('darkTint.amberSubtle'),
 'color.status.offline':P('graphite.500'),'color.status.paused':P('graphite.500'),
 'color.fill.quiet':P('whiteA.a08'),'color.fill.quieter':P('whiteA.a06'),'color.fill.placeholder':P('whiteA.a06'),'color.fill.placeholderEdge':P('whiteA.a08'),'color.fill.strong':P('graphite.50'),
 'color.traffic.edge':P('blackA.a25'),
 'color.tooltip.bg':P('inkDark.a95'),
 'color.glass.titleTab':P('whiteA.a12'),'color.glass.float':P('whiteA.a10'),'color.glass.bar':P('whiteA.a08'),'color.glass.highlight':P('whiteA.a10'),
 'shadow.window':[L(0,0,0,.5,P('whiteA.a14')),L(0,24,60,0,P('blackA.a50')),L(0,4,14,0,P('blackA.a30'))],
 'shadow.card':[L(0,0,0,.5,P('whiteA.a08'))],'shadow.paneOutline':[L(0,0,0,.5,P('whiteA.a10'),True)],
 'shadow.titleTab':[L(0,0,0,.5,P('whiteA.a10')),L(0,1,2,0,P('blackA.a30'))],
 'shadow.lift':[L(0,0,0,.5,P('whiteA.a14')),L(0,10,28,0,P('blackA.a45')),L(0,2,6,0,P('blackA.a25'))],
 'shadow.focusRing':[L(0,0,0,1.5,P('blue.500'))],
 'shadow.hoverRing':[L(0,0,0,1,P('whiteA.a20'))],
 'shadow.ghost':[L(0,8,24,0,P('blackA.a45')),L(0,0,0,.5,P('whiteA.a12'))],
 'shadow.tooltip':[L(0,4,12,0,P('blackA.a40'))],'shadow.float':[L(0,0,0,.5,P('whiteA.a14')),L(0,4,12,0,P('blackA.a30'))],
 'shadow.osSelected':[L(0,0,0,.5,P('whiteA.a12'))],'shadow.placeholder':[L(0,0,0,1,P('whiteA.a08'),True)],
 'shadow.glassHighlight':[L(0,.5,0,0,P('whiteA.a10'),True)],
 'opacity.paneUnfocusedContent':P('ratio.0_45'),
 'opacity.osTint':P('ratio.0_18'),
}

# ------------------------------------------------------------------ 3. COMPONENT (refs to semantic only)
R=lambda p:'{'+p+'}'
def ref(p,t=None):  # type inferred later
    return R(p)
COMP={
 'window':{'bg':'color.bg.window','tintOpacity':'opacity.windowTint','radius':'radius.window','shadow':'shadow.window','bodyPaddingRight':'space.8','bodyPaddingBottom':'space.8','gutter':'space.8'},
 'titlebar':{'height':'size.titlebar','tintOpacity':'opacity.titlebarTint','leftWidth':'size.titleLeft','leftPaddingX':'space.16','leftGap':'space.8',
   'trafficSize':'size.trafficLight','trafficClose':'color.traffic.close','trafficMinimize':'color.traffic.minimize','trafficZoom':'color.traffic.zoom','trafficEdge':'color.traffic.edge',
   'toggleIcon':'color.text.muted','tabsGap':'space.2','actionsGap':'space.3','actionsPaddingRight':'space.10'},
 'tab':{'height':'size.control','radius':'radius.control','paddingLeft':'space.10','paddingRight':'space.8','gap':'space.7','fontSize':'font.size.body',
   'text':'color.text.muted','textSelected':'color.text.primary','weightSelected':'font.weight.medium','bgSelected':'color.glass.titleTab','shadowSelected':'shadow.titleTab',
   'close':'color.text.tertiary','dotSize':'size.dotSm','addIcon':'color.text.secondary'},
 'titleAction':{'size':'size.control','radius':'radius.control','icon':'color.text.action','iconSize':'size.iconMd','bgOn':'color.state.onWindow','iconOn':'color.text.primary',
   'bgPrimary':'color.accent.default','iconPrimary':'color.text.onAccent','separator':'color.border.separatorWindow','separatorHeight':'size.separatorTall','separatorMarginX':'space.6'},
 'rail':{'width':'size.rail','gap':'space.6','paddingTop':'space.4','paddingBottom':'space.10','buttonSize':'size.railButton','buttonRadius':'radius.railButton',
   'icon':'color.text.muted','bgOn':'color.state.onWindow','iconOn':'color.text.primary'},
 'sider':{'width':'size.sider','bg':'color.surface.card','radius':'radius.card','shadow':'shadow.card','marginRight':'space.8',
   'headerPaddingTop':'space.16','headerPaddingRight':'space.12','headerPaddingBottom':'space.10','headerPaddingLeft':'space.16','titleSize':'font.size.bodyLg','titleWeight':'font.weight.semibold',
   'addButtonSize':'size.controlMd','addButtonRadius':'radius.controlSm','addButtonBg':'color.surface.control','addButtonIcon':'color.text.muted',
   'listPaddingX':'space.8','listPaddingY':'space.2',
   'footerBorder':'color.border.subtle','footerPaddingX':'space.16','footerPaddingY':'space.10','footerText':'color.text.secondary','footerSize':'font.size.small'},
 'search':{'height':'size.controlLg','radius':'radius.control','bg':'color.surface.sunken','paddingX':'space.9','gap':'space.6','marginX':'space.12','marginBottom':'space.8',
   'text':'color.text.secondary','fontSize':'font.size.body','kbdSize':'font.size.caption','kbdText':'color.text.tertiary','kbdBorder':'color.border.keycap','kbdBg':'color.surface.keycap','kbdRadius':'radius.keycap'},
 'groupHeader':{'height':'size.groupRow','radius':'radius.control','paddingX':'space.8','paddingTop':'space.12','paddingBottom':'space.5','gap':'space.4','fontSize':'font.size.footnote','weight':'font.weight.medium',
   'text':'color.text.secondary','count':'color.text.quaternary','countSize':'font.size.caption',
   'dropBg':'color.accent.tint08','dropText':'color.text.accent','dropCount':'color.accent.default',
   'expandTrack':'color.accent.tint15','expandFill':'color.accent.default','expandHeight':'size.progressThin','expandDelay':'motion.duration.autoExpand'},
 'siderItem':{'paddingTop':'space.6','paddingRight':'space.6','paddingBottom':'space.6','paddingLeft':'space.8','gap':'space.9','radius':'radius.control','spacing':'space.1',
   'bgHover':'color.state.hover','bgActive':'color.state.selectedAccent','bgSelected':'color.state.selected',
   'nameSize':'font.size.bodyLg','nameWeight':'font.weight.medium','nameLineHeight':'font.lineHeight.name','nameColor':'color.text.primary',
   'subSize':'font.size.footnote','subFamily':'font.family.mono','subLineHeight':'font.lineHeight.sub','subTracking':'font.tracking.mono','subColor':'color.text.secondary',
   'dotSize':'size.dot','dotHalo':'size.dotHalo','dotOnline':'color.success.default','dotOnlineHalo':'color.success.halo','dotOffline':'color.status.offline',
   'dotConnecting':'color.warning.default','dotConnectingHalo':'color.warning.halo','connectingText':'color.warning.text',
   'moreSize':'size.controlSm','moreRadius':'radius.controlSm','moreBg':'color.surface.controlStrong','moreIcon':'color.text.muted'},
 'osLogo':{'size':'size.osLogo','radius':'radius.control','glyphSize':'size.osGlyph','tintMix':'opacity.osTint','bgBase':'color.surface.card','bgSelected':'color.surface.card','shadowSelected':'shadow.osSelected'},
 'card':{'bg':'color.surface.card','radius':'radius.card','shadow':'shadow.card'},
 'pane':{'bg':'color.surface.terminal','radius':'radius.pane','outline':'shadow.paneOutline','gap':'space.8',
   'headerHeight':'size.paneHeader','headerPaddingLeft':'space.14','headerPaddingRight':'space.8','headerGap':'space.7','headerFontSize':'font.size.body',
   'name':'color.text.primary','nameWeight':'font.weight.semibold','host':'color.text.secondary','hostSize':'font.size.footnote','hostFamily':'font.family.mono',
   'buttonSize':'size.controlSm','buttonIcon':'color.icon.subtle','iconSize':'size.iconSm'},
 'paneFocus':{'ringFocused':'shadow.focusRing','ringHover':'shadow.hoverRing',
   'nameFocused':'color.text.primary','nameWeightFocused':'font.weight.semibold','nameUnfocused':'color.text.muted','nameWeightUnfocused':'font.weight.medium','hostUnfocused':'color.text.disabled',
   'contentOpacityUnfocused':'opacity.paneUnfocusedContent','contentSaturateUnfocused':'opacity.paneUnfocusedSaturate','statusOpacityUnfocused':'opacity.paneUnfocusedStatus',
   'contentOpacityHover':'opacity.paneHoverContent','contentSaturateHover':'opacity.paneHoverSaturate','buttonsOpacityUnfocused':'opacity.hidden',
   'cursorRingUnfocused':'color.text.secondary','transition':'motion.duration.focus',
   'kbdSize':'font.size.caption2','kbdWeight':'font.weight.medium','kbdRadius':'radius.chip','kbdPaddingX':'space.5','kbdPaddingY':'space.3',
   'kbdBg':'color.fill.quiet','kbdText':'color.text.secondary','kbdBgFocused':'color.accent.default','kbdTextFocused':'color.text.onAccent'},
 'activityBadge':{'fontSize':'font.size.caption','radius':'radius.badge','paddingLeft':'space.6','paddingRight':'space.7','paddingY':'space.2','gap':'space.4','dotSize':'size.dotSm',
   'outputText':'color.text.accent','outputBg':'color.accent.tint10','outputDot':'color.accent.default','bellText':'color.warning.text','bellBg':'color.warning.tint14'},
 'fadeBar':{'tint':'color.surface.terminal','extent':'size.fadeExtent','extentPane':'size.fadeExtentPane',
   'blur1':'glass.fade.blur1','blur2':'glass.fade.blur2','blur3':'glass.fade.blur3','mask1Solid':'glass.fade.mask1Solid','mask1End':'glass.fade.mask1End','mask2Solid':'glass.fade.mask2Solid','mask2End':'glass.fade.mask2End',
   'mask3Solid':'glass.fade.mask3Solid','mask3End':'glass.fade.mask3End','tintStart':'glass.fade.tintStart','tintMid':'glass.fade.tintMid','tintMidStop':'glass.fade.tintMidStop','tintEnd':'glass.fade.tintEnd'},
 'statusBar':{'height':'size.statusBar','heightPane':'size.paneStatusBar','paddingX':'space.14','gap':'space.14','gapPane':'space.10','fontSize':'font.size.footnote','text':'color.text.secondary',
   'hostFamily':'font.family.mono','hostColor':'color.text.primary','hostWeight':'font.weight.semibold','divider':'color.border.control','dividerOnGlass':'color.border.onGlass','dividerHeight':'size.separatorShort'},
 'badge':{'fontSize':'font.size.footnote','fontSizeCompact':'font.size.caption','paddingX':'space.8','paddingXCompact':'space.7','paddingY':'space.2','paddingYCompact':'space.1','radius':'radius.pill','gap':'space.5',
   'neutralBg':'color.surface.control','neutralText':'color.text.neutralBadge','neutralBgOnGlass':'color.fill.quiet',
   'okBg':'color.success.subtle','okText':'color.success.text','okDot':'color.success.default','okBgOnGlass':'color.success.onGlass',
   'runBg':'color.accent.subtle','runText':'color.text.accent'},
 'button':{'height':'size.control','radius':'radius.control','paddingX':'space.9','gap':'space.6','fontSize':'font.size.body','text':'color.text.strong',
   'border':'color.border.control','borderWidth':'size.divider','primaryBg':'color.accent.default','primaryText':'color.text.onAccent','bgHover':'color.state.hover','bgPressed':'color.state.pressed'},
 'iconButton':{'size':'size.controlMd','sizeSm':'size.controlSm','sizeXs':'size.controlXs','radius':'radius.controlSm','icon':'color.icon.default','iconSubtle':'color.icon.subtle','iconFaint':'color.icon.faint',
   'bg':'color.surface.control','bgHover':'color.state.hover','iconSize':'size.icon','stroke':'size.iconStroke'},
 'floatButton':{'size':'size.controlLg','bg':'color.surface.card','shadow':'shadow.floatButton','icon':'color.text.strong','primaryBg':'color.accent.default','primaryIcon':'color.text.onAccent'},
 'segmented':{'bg':'color.surface.control','radius':'radius.control','padding':'space.2','fontSize':'font.size.footnote','itemPaddingX':'space.9','itemPaddingY':'space.2','itemRadius':'radius.chip',
   'text':'color.text.muted','bgOn':'color.surface.raised','textOn':'color.text.primary','shadowOn':'shadow.segment'},
 'breadcrumb':{'height':'size.controlMd','radius':'radius.controlSm','bg':'color.surface.field','bgOnGlass':'color.fill.quieter','paddingX':'space.8','gap':'space.3','text':'color.text.muted','current':'color.text.primary','separator':'color.text.placeholder'},
 'fileList':{'headerHeight':'size.listRow','headerText':'color.text.secondary','headerSize':'font.size.footnote','headerBorder':'color.border.divider',
   'rowHeight':'size.listRow','rowRadius':'radius.controlSm','rowPaddingX':'space.8','rowGap':'space.2','fontSize':'font.size.body','zebra':'color.surface.zebra',
   'bgSelected':'color.state.selectedAccent','textSelected':'color.text.accentStrong','meta':'color.text.secondary','fileIcon':'color.icon.subtle','folderIcon':'color.icon.folder','folderFill':'color.icon.folderFill',
   'paneHeaderHeight':'size.fileHeader'},
 'progress':{'height':'size.progress','radius':'radius.bar','track':'color.border.subtle','fill':'color.accent.default','fillNeutral':'color.fill.strong','fillPaused':'color.status.paused','fillDone':'color.success.default'},
 'tooltip':{'bg':'color.tooltip.bg','text':'color.tooltip.text','kbd':'color.tooltip.kbd','fontSize':'font.size.footnote','paddingX':'space.8','paddingY':'space.4','radius':'radius.controlSm','shadow':'shadow.tooltip','arrow':'space.4'},
 'menu':{'bg':'color.surface.raised','radius':'radius.lift','shadow':'shadow.lift','itemHeight':'size.control','itemRadius':'radius.controlSm','itemPaddingX':'space.8','itemHover':'color.state.hover','itemHoverText':'color.text.primary','fontSize':'font.size.body','separator':'color.border.subtle'},
 'floatPill':{'bg':'color.glass.float','blur':'glass.float.blur','saturate':'glass.float.saturate','shadow':'shadow.float','radius':'radius.pill','text':'color.text.strong','fontSize':'font.size.footnote','paddingLeft':'space.8','paddingRight':'space.10','paddingY':'space.4'},
 'dragLift':{'threshold':'motion.drag.threshold','holdDelay':'motion.duration.hold','scale':'motion.drag.liftScale','bg':'color.surface.raised','radius':'radius.lift','shadow':'shadow.lift',
   'stackShadow':'shadow.liftStack','stackOffset':'space.5','stackInset':'space.6','stackBackOpacity':'opacity.liftStackBack','liftDuration':'motion.duration.lift',
   'sourceOpacity':'opacity.dragSource'},
 'dropIndicator':{'placeholderBg':'color.fill.placeholder','placeholderShadow':'shadow.placeholder','placeholderAccentBg':'color.accent.tint06','placeholderAccentShadow':'shadow.placeholderAccent',
   'lineColor':'color.accent.default','lineHeight':'size.dropLine','zoneBorder':'color.accent.default','zoneBorderWidth':'size.focusRing','zoneBg':'color.accent.tint035','zoneRadius':'radius.dropZone',
   'hintBg':'color.accent.subtle','hintText':'color.text.accent','flashBg':'color.accent.tint10','flashShadow':'shadow.flash','flashDuration':'motion.duration.flash','flashEasing':'motion.easing.out',
   'autoScrollZone':'motion.drag.autoScrollZone','autoScrollMax':'motion.drag.autoScrollMax','autoScrollTint':'color.accent.tint13','autoScrollEdge':'color.accent.tint50'},
 'dragGhost':{'bg':'color.surface.raised','radius':'radius.dropZone','shadow':'shadow.ghost','paddingLeft':'space.10','paddingRight':'space.12','paddingY':'space.7','fontSize':'font.size.body',
   'countBg':'color.accent.default','countText':'color.text.onAccent','countSize':'font.size.caption2'},
 'reorderMotion':{'displace':'motion.spring.displace','drop':'motion.spring.drop','cancel':'motion.spring.smooth','groupCollapse':'motion.duration.collapse','groupRestore':'motion.duration.restore',
   'tabDisplace':'motion.spring.displace','paneDisplace':'motion.spring.displace','paneDrop':'motion.spring.drop'},
}
# a few semantic tokens referenced by components that need adding
SEM['opacity']['hidden']=N(P('ratio.0'))
SEM['opacity']['osTint']=N(P('ratio.0_09'),'OS logo 底 = 品牌色 9% 混入卡片色')
DERIVED_COMP={'menu':'设计稿未画菜单，按浮起层/拖动卡片推导','reorderMotion.tabDisplace':'tab/窗格拖动未单独设计，沿用 sider 拖动参数',
  'reorderMotion.paneDisplace':'同上','reorderMotion.paneDrop':'同上','dropIndicator.lineColor':'MyGo ListState.Reorder 插入线','dropIndicator.lineHeight':'同上','button.bgHover':'未画','button.bgPressed':'未画','iconButton.bgHover':'未画'}

# Native desktop metrics absent from the supplied mockup tokens. Semantic values
# remain the source; UI extensions below only reference those semantic paths.
SEM['size'].update({
 'windowWidth':D(1180), 'windowHeight':D(780),
 'windowMinWidth':D(1000), 'windowMinHeight':D(640),
 'trafficLightY':D(17), 'siderMinWidth':D(236), 'siderMaxWidth':D(320),
 'paneMinWidth':D(280), 'paneMinHeight':D(160),
 'dragPreviewWidth':D(220), 'dragPreviewHeight':D(140),
 'tabDetachDistance':D(8), 'dragGrabX':D(30), 'dragGrabY':D(18),
 'paneEnterOffset':D(12), 'dividerHandleWidth':D(3),
 'dividerHandleHeight':D(28), 'dividerKeyboardStep':D(8),
 'previewInset':D(4), 'paneActionRevealPadding':D(40),
 'hostDialogWidth':D(340), 'groupDialogWidth':D(320),
 'portFieldWidth':D(90), 'dialogTitleSize':D(18),
 'tabContentHeight':D(20), 'tabStripHeight':D(36),
 'tabDragHandleInset':D(30), 'tabTitleOffset':D(-1.5), 'tabCountOffset':D(-.5),
 'scrollbarMinThumb':D(24), 'scrollbarRevealZone':D(24),
 'terminalContentBottom':D(50), 'emptyPaneTitleSize':D(15),
 'statsHeaderHeight':D(44), 'statsChartHeight':D(44,'CPU 趋势图'),
 'statsChartStroke':D(1.3), 'statsSectionGap':D(18),
 'statsNetChartHeight':D(20,'网络上下行迷你趋势'), 'statsValueLarge':D(28,'CPU 主数字'),
 'statsSkeletonLine':D(10,'进程 Top 骨架条'),
})
SEM['color']['fill']['shimmer']=C(P('gray.40'),'骨架占位流光高光')
DARK['color.fill.shimmer']=P('graphite.600')
SEM['motion']['duration']['shimmer']=MS(1400,'骨架占位流光一周')
SEM['motion']['duration']['liveBeat']=MS(900,'实时状态点每次采样的跳动')
SEM['opacity']['liveBeatScale']=N(1.35,'实时状态点跳动起始缩放')
SEM['opacity'].update({
 'dragRotation':N(-1), 'dragPreview':N(.9), 'previewHeader':N(.96),
 'paneCloseScale':N(.97), 'dropEdgeRatio':N(.3), 'paneActionTint':N(.9),
})
SEM['font']['weight']['emptyPaneTitle']=W(550)
SEM['font']['lineHeight']['keycap']=D(12)
SEM['motion']['duration'].update({
 'frameInterval':MS(16), 'zoom':MS(240), 'paneEnter':MS(220),
 'paneClose':MS(160), 'dividerFade':MS(120), 'sidebar':MS(220),
 'scrollbarHold':MS(700), 'scrollbarFade':MS(180),
 'instantNonzero':MS(.000001,'1 ns: MyGo zero Animation selects a default spring'),
})
SEM['motion']['spring']['zoom']=SP({'duration':240,'bounce':0})
SEM['motion']['easing']['pane']=BZ([.2,.8,.2,1])
SEM['color']['state']['tabHover']=C('rgba(255,255,255,.45)')
SEM['color']['state']['railHover']=C('rgba(0,0,0,.045)')
SEM['color']['state']['railPressed']=C(P('black.a12'))
SEM['color']['status']['scrollbar']=C('rgba(0,0,0,.32)')
SEM['color']['status']['dividerHandle']=C(P('gray.400'))
SEM['color']['status']['danger']=C('#dc2626')
DARK.update({
 'color.state.tabHover':P('whiteA.a10'),
 'color.state.railHover':P('whiteA.a06'),
 'color.state.railPressed':P('whiteA.a12'),
 'color.status.scrollbar':'rgba(255,255,255,.35)',
 'color.status.dividerHandle':P('graphite.500'),
 'color.status.danger':'#ef4444',
})
COMP['desktop']={
 **{k:'size.'+k for k in ('windowWidth','windowHeight','windowMinWidth','windowMinHeight','trafficLightY','siderMinWidth','siderMaxWidth','paneMinWidth','paneMinHeight','dragPreviewWidth','dragPreviewHeight','tabDetachDistance','dragGrabX','dragGrabY','paneEnterOffset','dividerHandleWidth','dividerHandleHeight','dividerKeyboardStep','previewInset')},
 **{k:'opacity.'+v for k,v in {'dragRotation':'dragRotation','dragOpacity':'dragPreview','previewHeaderOpacity':'previewHeader','paneCloseScale':'paneCloseScale','dropEdgeRatio':'dropEdgeRatio'}.items()},
 **{k:'motion.duration.'+v for k,v in {'frameInterval':'frameInterval','zoomDuration':'zoom','paneEnterDuration':'paneEnter','paneCloseDuration':'paneClose','dividerFadeDuration':'dividerFade','sidebarDuration':'sidebar','instantDuration':'instantNonzero'}.items()},
 'zoomSpring':'motion.spring.zoom', 'motionEasing':'motion.easing.pane',
 'dividerHandleColor':'color.status.dividerHandle','dividerHandleRadius':'size.focusRing',
 'trafficLightX':'space.12','siderKeyboardStep':'space.10',
 'railHover':'color.state.railHover','railPressed':'color.state.railPressed',
 'danger':'color.status.danger',
}
COMP['pane'].update({
 'frameInterval':'motion.duration.frameInterval',
 'emptyIconSize':'size.railButton','emptyTitleSize':'size.emptyPaneTitleSize',
 'emptyTitleWeight':'font.weight.emptyPaneTitle',
 'contentTop':'space.10','contentSide':'space.8','contentBottom':'size.terminalContentBottom',
 'actionTop':'space.6','actionRight':'space.8','actionWidth':'size.controlXs',
 'actionHeight':'size.controlSm','actionGap':'space.2',
 'actionRevealPadding':'size.paneActionRevealPadding','actionRadius':'radius.control',
 'actionBlur':'glass.float.blur','actionTint':'opacity.paneActionTint',
})
SEM['glass']['paneActionBlur']=D(4)
COMP['pane']['actionBlur']='glass.paneActionBlur'
COMP['paneFocus']['kbdLineHeight']='font.lineHeight.keycap'
COMP['activityBadge']['bellIconSize']='font.size.body'
COMP['scrollbar']={
 'holdDuration':'motion.duration.scrollbarHold','fadeDuration':'motion.duration.scrollbarFade',
 'minThumb':'size.scrollbarMinThumb','trackWidth':'space.12','thumbWidth':'space.6',
 'thumbInset':'space.3','endInset':'space.2','revealZone':'size.scrollbarRevealZone',
 'color':'color.status.scrollbar',
}
COMP['dialog']={
 'hostWidth':'size.hostDialogWidth','groupWidth':'size.groupDialogWidth',
 'portWidth':'size.portFieldWidth','titleSize':'size.dialogTitleSize',
}
COMP['stats']={
 'headerHeight':'size.statsHeaderHeight','chartHeight':'size.statsChartHeight',
 'chartStroke':'size.statsChartStroke','sectionGap':'size.statsSectionGap',
 'netChartHeight':'size.statsNetChartHeight','valueLarge':'size.statsValueLarge',
 'hostBg':'color.surface.sunken','hostRadius':'radius.card',
 'processBar':'color.accent.tint10','processRadius':'radius.controlSm',
 'skeleton':'color.border.subtle','shimmer':'color.fill.shimmer','shimmerPeriod':'motion.duration.shimmer',
 'skeletonLine':'size.statsSkeletonLine',
 'beatDuration':'motion.duration.liveBeat','beatScale':'opacity.liveBeatScale','beatEasing':'motion.easing.out',
}
COMP['tab'].update({
 'contentHeight':'size.tabContentHeight','stripHeight':'size.tabStripHeight',
 'dragHandleInset':'size.tabDragHandleInset','titleOffset':'size.tabTitleOffset',
 'countOffset':'size.tabCountOffset','bgHover':'color.state.tabHover',
})
# These are icon metrics, not rail-button or typography metrics. Preserve their
# existing desktop dimensions where the supplied component set had no token.
SEM['size']['emptyPaneIcon']=D(32)
SEM['size']['bellIcon']=D(13)
COMP['pane']['emptyIconSize']='size.emptyPaneIcon'
COMP['activityBadge']['bellIconSize']='size.bellIcon'

# The main catalog keeps a fixed drag-slot height. Render and destination math
# share these component metrics; other list padding follows the theme's sider.
SEM['size']['catalogRowHeight']=D(40)
SEM['color']['surface']['liftStack']=C('#f9f9fa')
DARK['color.surface.liftStack']=P('graphite.650')
COMP['catalog']={
 'rowHeight':'size.catalogRowHeight',
 'listPaddingX':'space.8','listPaddingY':'space.2',
 'rowGap':'space.2','groupGap':'space.10',
}
COMP['dragLift']['stackBg']='color.surface.liftStack'

# FTP v3: raw additions live in the palette; semantic and component additions
# only reference the preceding tier.
PALETTE['red']={
 '50':C('#fdecec','Apple systemRed 浅底',True),
 '500':C('#ff3b30','Apple systemRed',True),
 '700':C('#d70015','Apple systemRed 文字',True),
}
PALETTE['darkTint'].update({
 'red400':C('#ff453a','深色 danger',True),
 'red300':C('#ff6961','深色 danger 文字',True),
 'redSubtle':C('#3a1d1c','深色 danger 浅底',True),
})
for value in [10,42,50,80,90,100,120,140,160,180,240,300,320,384,420,720]:
 PALETTE['size'].setdefault(str(value),D(value))
PALETTE['duration'].update({'800':MS(800),'3000':MS(3000)})
PALETTE['ratio'].update({'0_9':N(.9),'0_96':N(.96)})
PALETTE['speed']['demoTransfer']=N(13002342.4,'演示传输字节/秒，12.4 MiB/s',True)
PALETTE['shadow']={
 'popover':SH([
  L(0,0,0,.5,A('0,0,0','.10')),L(0,10,28,0,A('0,0,0','.12')),
  L(0,2,6,0,A('0,0,0','.06')),L(0,.5,0,0,A('255,255,255','.90'),True),
 ],'传输浮层'),
 'popoverDark':SH([
  L(0,0,0,.5,A('255,255,255','.14')),L(0,10,28,0,A('0,0,0','.45')),
  L(0,2,6,0,A('0,0,0','.25')),
 ],'深色传输浮层',True),
}

SEM['color']['danger']={
 'default':C(P('red.500'),'失败圆点、失败图标',True),
 'text':C(P('red.700'),'失败文字',True),
 'subtle':C(P('red.50'),'失败图标底、失败徽章底',True),
}
SEM['color']['protocol']={
 'sftpBg':C(P('blue.50'),'SFTP 图标块底'),
 'sftpGlyph':C(P('blue.500'),'SFTP 字形'),
 'ftpBg':C(P('gray.100'),'FTP 图标块底'),
 'ftpGlyph':C(P('gray.600'),'FTP 字形'),
}
SEM['color']['fill']['quietHover']=C(P('black.a08'),'传输胶囊悬停')
SEM['radius']['popover']=D(P('radius.12'))
SEM['opacity']['revealFrom']=N(P('ratio.0_6'),'本地面板展开起始透明度')
SEM['opacity']['capsuleEnterScale']=N(P('ratio.0_9'))
SEM['opacity']['popoverEnterScale']=N(P('ratio.0_96'))
SEM['motion']['duration'].update({
 'exit':MS(P('duration.120'),'浮层退出、胶囊淡出'),
 'spin':MS(P('duration.800'),'连接中转圈一周'),
})
SEM['size'].update({
 'paneMinLocal':D(P('size.320')), 'paneMinRemote':D(P('size.420')),
 'popover':D(P('size.384')), 'capsuleBarWidth':D(P('size.32')),
 'capsuleBarHeight':D(P('size.3')), 'dockHeader':D(P('size.42')),
 'dockRow':D(P('size.50')), 'tabSpinner':D(P('size.10')),
 'fileToolbar':D(P('size.44')), 'fileCompactWidth':D(P('size.720')),
 'fileSearchWidth':D(P('size.160')), 'fileSizeWidth':D(P('size.80')),
 'fileDateWidth':D(P('size.140')), 'filePermissionsWidth':D(P('size.100')),
 'fileOwnerWidth':D(P('size.90')), 'fileGridCellWidth':D(P('size.120')),
 'fileGridCellHeight':D(P('size.100')), 'transferProgressWidth':D(P('size.180')),
 'transferSpeedWidth':D(P('size.120')), 'transferNameMinWidth':D(P('size.120')),
 'transferStatusWidth':D(P('size.80'),'传输状态列固定宽度',True),
 'splitGripHeight':D(P('size.32')), 'transferPopoverHeader':D(P('size.32')),
 'fileNameMinWidth':D(P('size.100')), 'fileGridIconSize':D(P('size.32')),
})
SEM['opacity']['splitDefaultRatio']=N(P('ratio.0_5'))
SEM['size']['transferDockMaxRows']=N(P('size.5'))
SEM['size']['transferPopoverMaxRows']=N(P('size.6'))
SEM['size']['transferConcurrency']=N(P('size.3'))
SEM['motion']['demoTransferSpeed']=N(P('speed.demoTransfer'),'演示传输吞吐',True)
SEM['shadow']['popover']=SH(P('shadow.popover'),'传输浮层')
DARK.update({
 'color.danger.default':P('darkTint.red400'),
 'color.danger.text':P('darkTint.red300'),
 'color.danger.subtle':P('darkTint.redSubtle'),
 'color.protocol.sftpBg':P('darkTint.blueSubtle'),
 'color.protocol.ftpBg':P('graphite.700'),
 'color.protocol.ftpGlyph':P('graphite.250'),
 'color.fill.quietHover':P('whiteA.a12'),
 'shadow.popover':P('shadow.popoverDark'),
})
COMP['protocolIcon']={
 'size':'size.osLogo','radius':'radius.control','glyph':'size.osGlyph',
 'sftpBg':'color.protocol.sftpBg','sftpGlyph':'color.protocol.sftpGlyph',
 'ftpBg':'color.protocol.ftpBg','ftpGlyph':'color.protocol.ftpGlyph',
}
COMP['connectionIndicator']={
 'offline':'color.status.offline','connected':'color.success.default',
 'connectedHalo':'color.success.halo','connecting':'color.warning.default',
 'connectingHalo':'color.warning.halo','halo':'size.dotHalo',
}
COMP['tab'].update({
 'dotConnected':'color.success.default','dotError':'color.danger.default',
 'ringDisconnected':'color.text.tertiary','ringWidth':'size.focusRing',
 'spinner':'color.warning.default','spinnerTrack':'color.warning.halo',
 'spinnerSize':'size.tabSpinner','spinnerStroke':'size.iconStroke',
 'spinnerPeriod':'motion.duration.spin','textDisconnected':'color.text.tertiary',
 'textErrorSelected':'color.danger.text','stateTransition':'motion.duration.focus',
})
COMP['paneToggle']={
 'size':'size.controlMd','radius':'radius.controlSm','icon':'color.text.muted',
 'bgHover':'color.state.hover','bgOn':'color.state.pressed','iconOn':'color.text.primary',
 'transition':'motion.duration.focus',
}
COMP['filePaneFocus']={
 'labelBgActive':'color.accent.subtle','labelTextActive':'color.text.accent',
 'labelWeightActive':'font.weight.semibold','labelTextInactive':'color.text.muted',
 'labelWeightInactive':'font.weight.medium','labelPaddingX':'space.8',
 'labelRadius':'radius.controlSm','selBgActive':'color.state.selectedAccent',
 'selTextActive':'color.text.accentStrong','selBgInactive':'color.state.selected',
 'selTextInactive':'color.text.primary','transition':'motion.duration.focus',
}
COMP['FTPSplit']={
 'gutter':'space.8','expand':'motion.spring.displace',
 'revealOpacityFrom':'opacity.revealFrom','crossfade':'motion.duration.focus',
 'minLocal':'size.paneMinLocal','minRemote':'size.paneMinRemote',
 'defaultRatio':'opacity.splitDefaultRatio','gripW':'space.4','gripH':'size.splitGripHeight',
 'grip':'color.text.placeholder','gripActive':'color.accent.default',
}
COMP['transferCapsule']={
 'height':'size.controlXs','radius':'radius.pill','bg':'color.fill.quiet',
 'bgHover':'color.fill.quietHover','bgOpen':'color.accent.subtle',
 'textOpen':'color.text.accent','text':'color.text.muted','arrow':'color.accent.default',
 'barWidth':'size.capsuleBarWidth','barHeight':'size.capsuleBarHeight',
 'barTrack':'color.accent.tint15','barFill':'color.accent.default',
 'doneArrow':'color.success.default','doneFill':'color.success.default',
 'error':'color.danger.text','enter':'motion.spring.smooth','exit':'motion.duration.exit',
 'enterScale':'opacity.capsuleEnterScale',
 'paddingLeft':'space.7','paddingRight':'space.9','gap':'space.6',
 'paddingX':'space.8','paddingY':'space.2',
 'fontSize':'font.size.footnote','weight':'font.weight.medium',
 'speedInterval':'motion.duration.autoExpand',
}
COMP['transferPopover']={
 'width':'size.popover','radius':'radius.popover','bg':'color.glass.float',
 'blur':'glass.float.blur','saturate':'glass.float.saturate','shadow':'shadow.popover',
 'offset':'space.8','enter':'motion.spring.smooth','exit':'motion.duration.exit',
 'enterScale':'opacity.popoverEnterScale','enterY':'space.6',
 'padding':'space.6','headerHeight':'size.transferPopoverHeader','maxRows':'size.transferPopoverMaxRows',
 'dismissY':'space.24',
 'rowHeight':'size.queueRow','exitY':'space.24',
}
COMP['transferDock']={
 'bg':'color.surface.card','radius':'radius.card','headerHeight':'size.dockHeader',
 'rowHeight':'size.dockRow','maxRows':'size.transferDockMaxRows',
 'grow':'motion.spring.smooth','collapse':'motion.spring.smooth',
 'concurrency':'size.transferConcurrency',
 'progressWidth':'size.transferProgressWidth','compactBreakpoint':'size.fileCompactWidth',
}
COMP['transferRow']={
 'upBg':'color.accent.subtle','upIcon':'color.accent.default',
 'downBg':'color.surface.control','downIcon':'color.text.muted',
 'doneBg':'color.success.subtle','doneIcon':'color.success.text',
 'errBg':'color.danger.subtle','errIcon':'color.danger.text',
 'speedText':'color.text.accent','errorText':'color.danger.text',
 'runBg':'color.accent.subtle','runText':'color.text.accent','runDot':'color.accent.default',
 'pausedBg':'color.surface.control','pausedText':'color.text.muted','pausedDot':'color.status.paused',
 'doneBadgeBg':'color.success.subtle','doneBadgeText':'color.success.text','doneDot':'color.success.default',
 'errBadgeBg':'color.danger.subtle','errBadgeText':'color.danger.text','errDot':'color.danger.default',
 'progressWidth':'size.transferProgressWidth','speedWidth':'size.transferSpeedWidth',
 'statusWidth':'size.transferStatusWidth',
 'nameMinWidth':'size.transferNameMinWidth','nameSize':'font.size.body',
 'pathSize':'font.size.caption','rowHeight':'size.queueRow',
 'detailSize':'font.size.caption','progressHeight':'size.progress',
 'gap':'space.8','paddingX':'space.12',
 'demoBytesPerSecond':'motion.demoTransferSpeed','speedInterval':'motion.duration.autoExpand',
 'transition':'motion.duration.focus',
}
COMP['filePane']={
 'headerHeight':'size.fileToolbar','statusHeight':'size.statusBar',
 'compactWidth':'size.fileCompactWidth','searchWidth':'size.fileSearchWidth',
 'sizeWidth':'size.fileSizeWidth','dateWidth':'size.fileDateWidth',
 'permissionsWidth':'size.filePermissionsWidth','ownerWidth':'size.fileOwnerWidth',
 'gridCellWidth':'size.fileGridCellWidth','gridCellHeight':'size.fileGridCellHeight',
 'rowInset':'space.8','toolGap':'space.4','contentPadding':'space.8',
 'nameMinWidth':'size.fileNameMinWidth','gridIconSize':'size.fileGridIconSize',
}
DERIVED_COMP.update({
 'filePane':'FTP v3 原生文件面板布局补全',
 'siderItem.bgActive':'用户新增：目录条目聚焦 active 底色，与打开的会话独立',
 'transferRow.progressWidth':'FTP v3 停靠传输进度列宽',
 'transferRow.speedWidth':'FTP v3 停靠传输速度列宽',
 'transferRow.statusWidth':'FTP v3 停靠传输状态列固定宽度',
 'transferRow.nameMinWidth':'FTP v3 传输名称最小宽度',
})

# deprecated: 侧栏不再显示状态/选中。保留生成以兼容已有主题；新 UI 不得使用。
DEPRECATED_COMP={
 **{'siderItem.'+key:'侧栏不再显示状态/选中' for key in (
  'bgSelected','dotSize','dotHalo','dotOnline','dotOnlineHalo','dotOffline',
  'dotConnecting','dotConnectingHalo','connectingText')},
 'osLogo.bgSelected':'侧栏不再显示状态/选中',
 'osLogo.shadowSelected':'侧栏不再显示状态/选中',
}
for key in ['dotOnline','dotConnecting','osSelected']:
 SEM['shadow'][key].desc += ' [deprecated: 侧栏不再显示状态/选中]'

# Shared desktop controls and motion. Keep framework behavior in the Base APIs;
# all application styling is resolved here before the MyGo adapter consumes it.
PALETTE['space']['20']=D(20)
PALETTE['easing']['inOut']=BZ([.42,0,.58,1])
PALETTE['spring']['content']=SP({'duration':350,'bounce':0})
SEM['space']['20']=D(P('space.20'))
SEM['color']['overlay']={'backdrop':C(P('blackA.a40'),'弹窗遮罩')}
SEM['color']['border']['controlHover']=C(P('gray.400'),'输入框悬停描边')
DARK['color.border.controlHover']=P('graphite.550')
SEM['shadow']['dialog']=SH([L(0,10,30,0,P('blackA.a30'))],'弹窗阴影')
DARK['shadow.dialog']=[L(0,10,30,0,P('blackA.a50'))]
SEM['motion']['easing']['search']=BZ(P('easing.inOut'))
SEM['motion']['spring']['content']=SP(P('spring.content'),'数字内容变化')
SEM['size']['tooltipMaxWidth']=D(P('size.320'),'提示最大宽度')
COMP['button'].update({'bg':'color.surface.control','transition':'motion.duration.focus'})
COMP['iconButton'].update({'bgPressed':'color.state.pressed','transition':'motion.duration.focus'})
for component in ['rail','siderItem','groupHeader','segmented']:
 COMP[component]['transition']='motion.duration.focus'
COMP['groupHeader']['countMotion']='motion.spring.content'
COMP['search'].update({'transition':'motion.duration.focus','easing':'motion.easing.search'})
COMP['input']={
 'height':'size.control','paddingX':'space.9','radius':'radius.control',
 'bg':'color.surface.control','text':'color.text.primary','fontSize':'font.size.body',
 'border':'color.border.control','borderHover':'color.border.controlHover',
 'borderFocus':'color.accent.default','borderWidth':'size.divider','transition':'motion.duration.focus',
}
COMP['segmented']['bgHover']='color.state.hover'
COMP['tooltip'].update({'maxWidth':'size.tooltipMaxWidth','offset':'space.6','railOffset':'space.8'})
COMP['dialog'].update({
 'bg':'color.surface.raised','backdrop':'color.overlay.backdrop','shadow':'shadow.dialog',
 'radius':'radius.card','padding':'space.20','gap':'space.12','viewportInset':'space.20',
})

# Compact Shell drag preview and reveal at the final pane size.
PALETTE['blur']['8']=D(8)
SEM['size']['previewLineHeight']=D(P('size.6'))
SEM['size']['previewLineLong']=N(P('ratio.0_78'))
SEM['size']['previewLineMedium']=N(P('ratio.0_6'))
SEM['size']['previewLineShort']=N(P('ratio.0_45'))
SEM['color']['fill']['skeleton']=C(P('gray.175'))
DARK['color.fill.skeleton']=P('graphite.550')
SEM['glass']['paneRevealBlur']=D(P('blur.8'))
SEM['motion']['duration']['paneReveal']=MS(P('duration.220'))
COMP['dragGhost'].update({
 'width':'size.dragPreviewWidth','height':'size.dragPreviewHeight','headerHeight':'size.control',
 'lineHeight':'size.previewLineHeight','lineRadius':'radius.bar','lineColor':'color.fill.skeleton',
 'lineLong':'size.previewLineLong','lineMedium':'size.previewLineMedium','lineShort':'size.previewLineShort',
})
COMP['paneReveal']={'blur':'glass.paneRevealBlur','duration':'motion.duration.paneReveal','easing':'motion.easing.out'}

SEM['motion']['duration']['connectionStage']=MS(P('duration.300'),'SSH 演示连接每个阶段的停留时间')
COMP['connectionIndicator']['stageDuration']='motion.duration.connectionStage'

# Session forms and settings; components reference semantic values only.
PALETTE['accentChoice']={
 'purple':C('#8e5cf7'), 'pink':C('#ec4899'), 'orange':C('#ff9500'), 'yellow':C('#f5b400'),
}
PALETTE['blueA']['a24']=C(A('10,132,255','.24'))
SEM['color']['accentChoices']={
 'blue':C(P('blue.500')), 'purple':C(P('accentChoice.purple')), 'pink':C(P('accentChoice.pink')), 'red':C(P('red.500')),
 'orange':C(P('accentChoice.orange')), 'yellow':C(P('accentChoice.yellow')), 'green':C(P('green.500')), 'graphite':C(P('gray.600')),
}
for leaf in SEM['color']['danger'].values(): leaf.derived=False
SEM['color']['danger']['tint14']=C(A('255,59,48','.14'),'错误输入框外光晕')
SEM['color']['scrim']=C(P('black.a18'),'会话弹窗遮罩，覆盖标题栏')
DARK.update({'color.danger.tint14':A('255,69,58','.22'),'color.scrim':P('blackA.a45'),'color.accent.tint16':P('blueA.a24')})
SEM['shadow'].update({
 'modal':SH([L(0,0,0,.5,P('black.a12')),L(0,24,64,0,A('0,0,0','.22')),L(0,4,16,0,P('black.a08'))],'会话弹窗'),
 'inputFocus':SH([L(0,0,0,1.5,P('blue.500'),True),L(0,0,0,3,P('blueA.a16'))],'输入框焦点：内环与外光晕'),
 'inputError':SH([L(0,0,0,1.5,P('red.500'),True),L(0,0,0,3,A('255,59,48','.14'))],'输入框错误：优先于焦点'),
 'switchKnob':SH([L(0,1,2,0,A('0,0,0','.2'))],'开关滑块'),
 'themeSelected':SH([L(0,0,0,3,P('blueA.a16'))],'外观主题卡片选中光晕'),
})
DARK.update({
 'shadow.modal':[L(0,0,0,.5,P('whiteA.a12')),L(0,24,64,0,A('0,0,0','.55')),L(0,4,16,0,P('blackA.a30'))],
 'shadow.inputFocus':[L(0,0,0,1.5,P('blue.500'),True),L(0,0,0,3,P('blueA.a24'))],
 'shadow.inputError':[L(0,0,0,1.5,P('darkTint.red400'),True),L(0,0,0,3,A('255,69,58','.22'))],
 'shadow.themeSelected':[L(0,0,0,3,P('blueA.a24'))],
})
SEM['opacity'].update({'disabled':N(.4),'modalEnterScale':N(.96),'modalExitScale':N(.98)})
SEM['size'].update({
 'modal':D(560),'formLabel':D(72),'settingsContent':D(720),'modalTopRatio':N(.12),'modalTopMin':D(72),'modalIconTile':D(32),
 'inputMonoSize':D(12),'formErrorIcon':D(12),'testSpinner':D(13),'collapseChevron':D(12),'collapseChevronStroke':D(2.25),'collapseChevronRotate':D(90),
 'switchWidth':D(32),'switchHeight':D(19),'switchKnob':D(15),'settingsHeaderPaddingX':D(32),'settingsRowHeight':D(48),'settingsFadeHeight':D(40),
 'schemePreviewHeight':D(66),'schemePreviewFont':D(10),'schemePreviewLineHeight':D(16),'schemeBadgeIcon':D(10),
 'themeAspect':N(1.6),'themeRadio':D(14),'themeRadioRing':D(4.5),'accentSwatch':D(20),'accentRing':D(1.5),'accentRingGap':D(2),
})
SEM['font']['tracking']['inputMono']=D(-.1)
SEM['motion']['duration'].update({'test':MS(10000),'settingsSwitchOut':MS(80)})
SEM['motion']['easing']['in']=BZ([.42,0,1,1])
COMP['modal']={
 'width':'size.modal','radius':'radius.window','bg':'color.surface.card','shadow':'shadow.modal','scrim':'color.scrim',
 'topRatio':'size.modalTopRatio','topMin':'size.modalTopMin','bottomMargin':'space.24',
 'headerPaddingTop':'space.18','headerPaddingRight':'space.16','headerPaddingBottom':'space.14','headerPaddingLeft':'space.24',
 'headerGap':'space.12','iconTile':'size.modalIconTile',
 'titleSize':'font.size.title','titleWeight':'font.weight.semibold','subtitleSize':'font.size.footnote','subtitle':'color.text.secondary',
 'bodyPaddingX':'space.24','divider':'color.border.divider',
 'footerPaddingTop':'space.14','footerPaddingRight':'space.16','footerPaddingBottom':'space.16','footerPaddingLeft':'space.24',
 'footerGap':'space.8','buttonPaddingX':'space.12','enterScale':'opacity.modalEnterScale','exitScale':'opacity.modalExitScale','enter':'motion.spring.smooth',
 'fade':'motion.duration.focus','exit':'motion.duration.exit','reduceFade':'motion.duration.focus',
}
COMP['input'].update({
 'gap':'space.7','bg':'color.surface.card','borderHover':'color.border.separatorWindow','focusShadow':'shadow.inputFocus','errorShadow':'shadow.inputError',
 'placeholder':'color.text.placeholder','icon':'color.text.tertiary','monoFont':'font.family.mono','monoSize':'size.inputMonoSize','monoTracking':'font.tracking.inputMono',
 'disabledBg':'color.surface.field','disabledText':'color.text.disabled','disabledBorder':'color.border.subtle','caret':'color.accent.default',
 'chipHeight':'size.controlXs','chipRadius':'radius.chip','chipBg':'color.surface.control','chipFontSize':'font.size.footnote',
})
COMP['formRow']={
 'labelWidth':'size.formLabel','labelGap':'space.12','rowGap':'space.12','label':'color.text.muted','labelSize':'font.size.body','labelHeight':'size.control',
 'error':'color.danger.text','errorSize':'font.size.footnote','errorIcon':'size.formErrorIcon','errorGap':'space.5',
 'description':'color.text.tertiary','descriptionSize':'font.size.footnote','descGap':'space.4',
}
COMP['testResult']={
 'gap':'space.6','fontSize':'font.size.small','icon':'size.iconSm','spinnerSize':'size.testSpinner','testing':'color.text.secondary',
 'success':'color.success.text','successIcon':'color.success.default','fail':'color.danger.text','failIcon':'color.danger.default',
 'info':'color.text.tertiary','infoSize':'size.formErrorIcon','crossfade':'motion.duration.focus',
}
COMP['collapse']={
 'headerHeight':'size.titlebar','chevron':'size.collapseChevron','chevronStroke':'size.collapseChevronStroke','chevronColor':'color.text.secondary',
 'title':'color.text.primary','titleSize':'font.size.bodyLg','titleWeight':'font.weight.semibold','summary':'color.text.tertiary',
 'summarySize':'font.size.small','duration':'motion.duration.collapse','chevronRotate':'size.collapseChevronRotate',
}
COMP['switch']={
 'w':'size.switchWidth','h':'size.switchHeight','knobSize':'size.switchKnob','inset':'space.2','off':'color.border.keycap','on':'color.accent.default',
 'knob':'color.text.onAccent','knobShadow':'shadow.switchKnob','animation':'motion.spring.smooth',
}
COMP['settings']={
 'navWidth':'size.sider','navItemHeight':'size.modalIconTile','navItemRadius':'radius.control','navItemOn':'color.accent.subtle',
 'navTextOn':'color.text.accentStrong','navIconOn':'color.accent.default','navIconTile':'size.controlXs',
 'headerPaddingTop':'space.16','headerPaddingRight':'space.14','headerPaddingBottom':'space.14','headerPaddingLeft':'size.settingsHeaderPaddingX',
 'titleSize':'font.size.title','titleWeight':'font.weight.semibold','subtitleSize':'font.size.footnote','subtitle':'color.text.secondary','close':'size.control',
 'contentMax':'size.settingsContent','contentPaddingY':'space.20','contentPaddingX':'size.settingsHeaderPaddingX',
 'groupTitleSize':'font.size.small','groupTitleWeight':'font.weight.semibold','groupTitle':'color.text.muted','groupTitleGap':'space.8',
 'groupGap':'space.22','groupBg':'color.surface.zebra','groupRadius':'radius.card','groupBorder':'color.border.raised','groupBorderWidth':'size.hairline',
 'rowMinHeight':'size.settingsRowHeight','rowPaddingY':'space.9','rowPaddingX':'space.14','rowDivider':'color.border.subtle','rowDividerInset':'space.14',
 'rowLabelSize':'font.size.bodyLg','rowLabel':'color.text.primary','rowDescSize':'font.size.footnote','rowDesc':'color.text.secondary',
 'groupFootnoteSize':'font.size.footnote','groupFootnote':'color.text.tertiary','fadeHeight':'size.settingsFadeHeight',
 'enterY':'space.8','exitY':'space.4','enter':'motion.spring.smooth','fade':'motion.duration.focus','exit':'motion.duration.exit',
 'switchY':'space.4','switchFadeOut':'motion.duration.settingsSwitchOut','switchFadeIn':'motion.duration.exit',
}
COMP['schemeCard']={
 'radius':'radius.lift','padding':'space.5','border':'color.surface.controlStrong','borderWidth':'size.divider','selected':'color.accent.default','selectedWidth':'size.progressThin',
 'previewHeight':'size.schemePreviewHeight','previewRadius':'radius.controlSm','previewFontSize':'size.schemePreviewFont','previewLineHeight':'size.schemePreviewLineHeight',
 'stripeHeight':'size.progress','stripeRadius':'radius.hair','stripeGap':'space.2','nameSize':'font.size.small','nameWeight':'font.weight.medium',
 'badgeSize':'font.size.caption2','badgeWeight':'font.weight.semibold','badgeBg':'color.accent.subtle','badgeText':'color.text.accent','badgeIcon':'size.schemeBadgeIcon',
}
COMP['themeCard']={
 'aspect':'size.themeAspect','radius':'radius.lift','border':'color.surface.controlStrong','borderWidth':'size.divider',
 'selected':'color.accent.default','selectedWidth':'size.progressThin','selectedShadow':'shadow.themeSelected','radio':'size.themeRadio','radioRing':'size.themeRadioRing',
}
COMP['accentSwatch']={'size':'size.accentSwatch','gap':'space.10','ringGap':'size.accentRingGap','ringWidth':'size.accentRing','ringInset':'color.surface.card'}

# Key generation sheet and protocol-switch metrics used by the session forms.
SEM['size']['credentialModal']=D(P('size.420'))
SEM['opacity']['collapseContentFadeFactor']=N(1.6)
COMP['modal'].update({'generateWidth':'size.credentialModal','protocolRadius':'radius.lift','protocolPadding':'space.3','protocolIcon':'size.controlXs'})
COMP['collapse']['contentFadeFactor']='opacity.collapseContentFadeFactor'
COMP['input']['selectWidth']='size.transferProgressWidth'
COMP['input'].update({'portWidth':'size.formLabel','numberWidth':'size.formLabel'})
PALETTE['lineHeight']['18']=D(18)
SEM['font']['lineHeight']['input']=D(P('lineHeight.18'),'统一空输入与中文占位文字行高，避免字体回退造成基线偏移')
COMP['input']['lineHeight']='font.lineHeight.input'
COMP['search']['lineHeight']='font.lineHeight.input'
SEM['size'].update({'settingsSelect':D(220),'settingsNumber':D(64),'settingsScrollback':D(96)})
COMP['settings'].update({'selectWidth':'size.settingsSelect','numberWidth':'size.settingsNumber','scrollbackWidth':'size.settingsScrollback'})

# Shared key hints. Flat hints retain the existing search-field text geometry.
SEM['size']['keycap']=D(P('size.18'))
PALETTE['white']['a75']=C(A('255,255,255','.75'),'设计指定的既有白色透明度',True)
SEM['color']['keycap']={'inkBg':C(P('whiteA.a16')),'inkText':C(P('white.a75')),'accentBg':C(P('whiteA.a20'))}
COMP['kbd']={
 'height':'size.keycap','minWidth':'size.keycap','paddingX':'space.5','radius':'radius.keycap','fontSize':'font.size.caption',
 'bg':'color.surface.keycap','border':'color.border.keycap','text':'color.text.tertiary',
 'inkBg':'color.keycap.inkBg','inkText':'color.keycap.inkText','accentBg':'color.keycap.accentBg','accentText':'color.text.onAccent',
}

# Original UIKit component specifications. All component fields reference the
# semantic layer; new metrics do not introduce a second palette.
SEM['size'].update({
 'buttonLarge':D(P('size.32')), 'badgeSmall':D(P('size.18')), 'badgeMedium':D(20),
 'check':D(P('size.14')), 'radioDot':D(4.5), 'panelHeader':D(46), 'toast':D(340),
 'emptyMaxWidth':D(220), 'keyValueRow':D(P('size.22')), 'sparklineHeight':D(P('size.34')),
 'sparklineStroke':D(P('size.1_5')), 'meterHeight':D(P('size.4')), 'iconXs':D(P('size.12')),
 'segmentedItem':D(P('size.24')), 'latencyWarn':N(150), 'latencyBad':N(400),
 'tableNameMinWidth':D(160), 'sparklineDotRadius':D(2.5,'趋势图末点'), 'sparklineHaloRadius':D(5,'趋势图末点光圈'),
})
SEM['opacity']['sparklineFill']=N(.20,'趋势图渐变填充顶部不透明度')
SEM['opacity']['sparklineHalo']=N(.18)
SEM['size'].update({'meterWarnRatio':N(.70),'meterCriticalRatio':N(.90)})
SEM['motion']['duration'].update({'fast':MS(P('duration.120')), 'base':MS(P('duration.160')), 'autoHide':MS(3000)})
SEM['color']['accent'].update({'hover':C(P('blue.600')), 'pressed':C(P('blue.700'))})
SEM['color']['danger'].update({'hover':C(P('red.500')), 'pressed':C(P('red.500'))})
SEM['color']['toast']={
 'action':C(P('darkTint.blue300')), 'ok':C(P('darkTint.green400')), 'error':C(P('darkTint.red400')),
 'warning':C(P('darkTint.amber400')),
}
SEM['shadow']['controlFocus']=SH([L(0,0,0,1.5,P('blue.500')),L(0,0,0,5.5,P('blueA.a16'))])
SEM['shadow']['cardInset']=SH([L(0,0,0,.5,P('black.a06'),True)],'UIKit 卡片内描边，外框几何不变')
DARK.update({
 'color.accent.hover':P('darkTint.blue300'), 'color.accent.pressed':P('darkTint.blue200'),
 'color.danger.hover':P('darkTint.red400'), 'color.danger.pressed':P('darkTint.red300'),
 'shadow.controlFocus':[L(0,0,0,1.5,P('blue.500')),L(0,0,0,5.5,P('blueA.a24'))],
 'shadow.cardInset':[L(0,0,0,.5,P('whiteA.a08'),True)],
})
COMP['card']['shadow']='shadow.cardInset'
COMP['button'].update({
 'heightSm':'size.controlSm','heightLg':'size.buttonLarge','paddingXSm':'space.9','paddingXLg':'space.14',
 'secondaryBg':'color.surface.control','secondaryBgHover':'color.surface.controlStrong','plainBgHover':'color.state.hover',
 'primaryBgHover':'color.accent.hover','primaryBgPressed':'color.accent.pressed',
 'dangerBg':'color.danger.default','dangerBgHover':'color.danger.hover','dangerBgPressed':'color.danger.pressed',
 'dangerText':'color.text.onAccent','dangerPlainText':'color.danger.text','dangerPlainBgHover':'color.danger.subtle',
 'kbdGap':'space.6','focusShadow':'shadow.controlFocus',
})
COMP['iconButton'].update({
 'bgOn':'color.state.pressed','iconOn':'color.text.primary','bgFilled':'color.surface.control','iconHover':'color.text.primary',
 'iconSizeSm':'size.iconSm','iconSizeXs':'size.iconXs','focusShadow':'shadow.controlFocus',
})
COMP['badge'].update({
 'heightSm':'size.badgeSmall','heightMd':'size.badgeMedium',
 'warnBg':'color.warning.subtle','warnText':'color.warning.textStrong','warnDot':'color.warning.default',
 'errBg':'color.danger.subtle','errText':'color.danger.text','errDot':'color.danger.default',
 'runDot':'color.accent.default','neutralDot':'color.status.offline','mutedBg':'color.surface.field','mutedText':'color.text.tertiary',
})
COMP['divider']={'color':'color.border.divider','width':'size.divider','verticalHeight':'size.separatorShort'}
COMP['spinner']={'sizeXs':'size.iconXs','sizeSm':'size.iconSm','sizeMd':'size.icon','color':'color.icon.default','period':'motion.duration.spin'}
COMP['check']={'size':'size.check','radius':'radius.keycap','border':'color.border.keycap','on':'color.accent.default','mark':'color.text.onAccent','radioDot':'size.radioDot'}
COMP['segmented'].update({'countText':'color.text.tertiary','countTextOn':'color.text.muted','itemHeightMd':'size.segmentedItem','heightSm':'size.controlSm'})
COMP['banner']={
 'paddingX':'space.12','paddingY':'space.9','radius':'radius.railButton','gap':'space.8','icon':'size.iconSm',
 'neutralBg':'color.surface.control','neutralText':'color.text.strong','neutralIcon':'color.text.secondary',
 'infoBg':'color.accent.subtle','infoText':'color.text.accentStrong','infoIcon':'color.accent.default',
 'warnBg':'color.warning.subtle','warnText':'color.warning.textStrong','warnIcon':'color.warning.default',
 'dangerBg':'color.danger.subtle','dangerText':'color.danger.text','dangerIcon':'color.danger.default',
}
COMP['panel']={
 'width':'size.statsPanel','headerHeight':'size.panelHeader','paddingX':'space.14','sectionGap':'space.8',
 'divider':'color.border.divider','statLabel':'color.text.primary','statValue':'color.text.secondary','caption':'color.text.tertiary',
}
COMP['sparkline']={'height':'size.sparklineHeight','stroke':'size.sparklineStroke','line':'color.accent.default','fillAlpha':'opacity.sparklineFill',
 'grid':'color.border.divider','dotRadius':'size.sparklineDotRadius','haloRadius':'size.sparklineHaloRadius','haloAlpha':'opacity.sparklineHalo'}
COMP['keyValue']={'rowHeight':'size.keyValueRow','key':'color.text.secondary','value':'color.text.primary','valueDim':'color.text.tertiary','font':'font.size.footnote'}
COMP['emptyState']={
 'iconTile':'size.titlebar','iconRadius':'radius.window','iconBg':'color.surface.control','icon':'color.text.secondary',
 'title':'font.size.bodyLg','detail':'color.text.secondary','maxWidth':'size.emptyMaxWidth',
}
COMP['toast']={
 'width':'size.toast','radius':'radius.card','bg':'color.tooltip.bg','text':'color.tooltip.text','detail':'color.tooltip.kbd',
 'action':'color.toast.action','okIcon':'color.toast.ok','errIcon':'color.toast.error','warnIcon':'color.toast.warning','shadow':'shadow.lift',
 'paddingX':'space.14','paddingY':'space.10','gap':'space.8','timeout':'motion.duration.autoHide',
}
COMP['popover']={'radius':'radius.window','padding':'space.12','bg':'color.surface.raised','shadow':'shadow.lift'}
COMP['latency']={'warnMs':'size.latencyWarn','badMs':'size.latencyBad','normal':'color.text.secondary','warn':'color.warning.text','bad':'color.danger.text'}
COMP['listRow']={'height':'size.listRow','radius':'radius.controlSm','paddingX':'space.8','gap':'space.9','meta':'color.text.secondary'}
COMP['breadcrumb']['iconSize']='size.iconXs'
COMP['fileList'].update({'headerBg':'color.surface.card','columnGap':'space.16','nameMinWidth':'size.tableNameMinWidth'})
COMP['progress'].update({'heightMeter':'size.meterHeight','meterFill':'color.accent.default','meterWarnAt':'size.meterWarnRatio','meterCritAt':'size.meterCriticalRatio','fillError':'color.danger.default'})
COMP['paneHeader']={
 'heightCompact':'size.paneHeader','heightToolbar':'size.fileHeader','heightTitle':'size.panelHeader',
 'titleFont':'font.size.bodyLg','titleWeight':'font.weight.semibold','paddingLeft':'space.14','paddingRight':'space.8','gap':'space.7',
}
COMP['paneFooter']={'height':'size.paneStatusBar','paddingX':'space.14','gap':'space.10','text':'color.text.secondary','font':'font.size.footnote','divider':'color.border.divider'}
COMP['filePaneFocus'].update({'borderFocused':'color.accent.default','borderWidthFocused':'size.focusRing'})
SEM['motion']['drag'].update({'standardScale':N(1.02),'standardRotation':N(-.6)})
COMP['dragLift'].update({'standardScale':'motion.drag.standardScale','standardRotation':'motion.drag.standardRotation'})

# MCP management page. Each table's header and rows share these column widths;
# its compact dialogs share one width.
SEM['size'].update({
 'mcpTableMinWidth':D(1024),'mcpNameColumn':D(210),'mcpBadgeColumn':D(86),'mcpScopeColumn':D(130),
 'mcpTimeColumn':D(150),'mcpKeysColumn':D(115),'mcpAuditTimeColumn':D(72),'mcpAuditKeyColumn':D(148),
 'mcpAuditConnectionColumn':D(120),'mcpAuditActionColumn':D(96),'mcpAuditStatusColumn':D(104),'mcpAuditDurationColumn':D(56),
 'mcpPolicyModal':D(720),'mcpKeyModal':D(680),'mcpDialog':D(440),
 'mcpDrawerMinWidth':D(340),'mcpDrawerWidth':D(500),'mcpMainMinWidth':D(360),
 'mcpRuleModeWidth':D(120,'命令规则匹配方式选择框'),'mcpPermissionColumn':D(118,'连接授权矩阵能力列'),
})
COMP['mcp']={
 'tableMinWidth':'size.mcpTableMinWidth',
 'keyName':'size.mcpNameColumn','keyState':'size.mcpBadgeColumn','keyDirs':'size.mcpNameColumn','keyUsed':'size.mcpTimeColumn',
 'policyName':'size.mcpNameColumn','policyCategory':'size.mcpBadgeColumn','policyConnections':'size.mcpScopeColumn',
 'policyKeys':'size.mcpKeysColumn','policyState':'size.mcpScopeColumn',
 'auditTime':'size.mcpAuditTimeColumn','auditKey':'size.mcpAuditKeyColumn','auditConnection':'size.mcpAuditConnectionColumn',
 'auditAction':'size.mcpAuditActionColumn','auditStatus':'size.mcpAuditStatusColumn','auditDuration':'size.mcpAuditDurationColumn',
 'policyModalWidth':'size.mcpPolicyModal','keyModalWidth':'size.mcpKeyModal','dialogWidth':'size.mcpDialog',
 'drawerMinWidth':'size.mcpDrawerMinWidth','drawerWidth':'size.mcpDrawerWidth','mainMinWidth':'size.mcpMainMinWidth',
 'ruleModeWidth':'size.mcpRuleModeWidth','permissionColumn':'size.mcpPermissionColumn',
}

# Bordered panels set into dialog and card bodies: rules, tasks, hit records.
COMP['inset']={
 'padding':'space.10','radius':'radius.control','borderWidth':'size.hairline',
 'compactPaddingX':'space.8','compactPaddingY':'space.6','compactRadius':'radius.controlSm',
 'bg':'color.surface.sunken','border':'color.border.subtle','raisedBg':'color.surface.card',
 'warningBg':'color.warning.subtle','warningBorder':'color.warning.default',
 'dangerBg':'color.danger.subtle','dangerBorder':'color.danger.default',
}

# A glyph or logo on a rounded fill: beside a line of text, in a list row, or
# beside a title.
COMP['iconTile']={
 'sizeSm':'size.controlSm','radiusSm':'radius.controlSm',
 'size':'size.control','radius':'radius.control',
 'sizeLg':'size.modalIconTile','radiusLg':'radius.railButton',
 'bg':'color.surface.control',
}
