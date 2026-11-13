from lib import *
MX,MW,RX,RW=308,792,1100,340
DEFAULT_TREE=[
 (0,"inbox","Inbox","3"),(0,"folder-o","Daily",None),(1,"file","2026-11-13",None),(1,"file","2026-11-12",None),
 (0,"folder-o","Campaign",None),(1,"query","Campaign locations",None),(1,"folder-c","NPCs",None),(1,"folder-o","Locations",None),
 (2,"file","The old harbour",None),(2,"file","Watchtower",None),(2,"file","Ferry schedule",None),
 (1,"folder-c","Sessions",None),(0,"folder-c","Literature",None),(0,"folder-c","Projects",None),
 (0,"folder-c","Docs",None),(0,"folder-c","Templates",None),(0,"file","Welcome",None),
]
def rail(f, active="book"):
    f.rect(0,0,52,900,fill=SIDE,name="Vesper rail")
    f.line(52,0,52,900,SUBTLE)
    f.dot(13,12,26,DEEP); f.add({"type":"shape","shape":"star5","x":19,"y":18,"w":14,"h":14,"fill":STAR})
    y=56
    for ic in ["chat","board","book","table"]:
        if ic==active: f.rect(10,y-6,32,28,fill=SEL,r=7); f.rect(0,y-2,3,20,fill=ACC,r=1)
        f.icon(ic,18,y,TEXT if ic==active else TER); y+=40
    f.line(16,y-4,36,y-4,BORDER)
    y+=8
    for ic in ["search","graph","calendar","inbox","upload"]:
        f.icon(ic,18,y,TER); y+=36
    f.icon("settings",18,826,TER)
    f.dot(13,856,26,"#3B3260"); f.t(21,860,"L",12,TEXT,600,w=12)
def explorer(f, active="The old harbour", tree=None, view="yours", footer=True, bg=True, body=True):
    if bg: f.rect(52,0,256,900,fill=SIDE,name="Explorer")
    if not body:
        return explorer_hl(f,active,tree)
    f.line(308,0,308,900,SUBTLE)
    f.t(68,12,"Knowledge",14,TEXT,600)
    for i,ic in enumerate(["pen","folder","more"]): f.icon(ic,236+i*22,13,TER)
    # segmented view switch
    f.rect(64,44,232,28,fill=SUNK,stroke=SUBTLE,r=7)
    if view=="yours": f.rect(66,46,84,24,fill=SEL,r=6)
    else: f.rect(152,46,142,24,fill=SEL,r=6)
    f.t(66,50,"Yours",12,TEXT if view=="yours" else TER,500,w=84,align="center")
    f.icon("bot",166,50,TEXT if view!="yours" else TER,14); f.t(186,50,"Agent knowledge",12,TEXT if view!="yours" else TER,500,w=106)
    f.rect(64,84,232,30,fill=SURF,stroke=BORDER,r=7)
    f.icon("search",74,91,TER,15); f.t(96,90,"Search pages, #tags, props",12,TER)
    f.kbd(264,90,"⌘O")
    y=128
    for lvl,kind,name,badge in (tree or DEFAULT_TREE):
        x=66+lvl*16
        if name==active:
            f.rect(60,y-3,240,26,fill=SEL,r=6); f.rect(60,y+2,2,16,fill=ACC,r=1)
        if kind.startswith("folder"):
            f.icon("chev-d" if kind=="folder-o" else "chev-r",x,y+2,FAINT,14)
            f.icon("folder",x+16,y+2,SEC if name!=active else TEXT,14)
            f.t(x+36,y,name,13,TEXT2 if name!=active else TEXT,500)
        elif kind=="inbox":
            f.icon("inbox",x+16,y+2,SEC,14); f.t(x+36,y,name,13,TEXT2,500)
        elif kind=="query":
            f.icon("table",x+16,y+2,FAINT if name!=active else ACCT,14)
            f.t(x+36,y,name,13,SEC if name!=active else TEXT,400 if name!=active else 500)
        elif kind=="bot":
            f.icon("bot",x+16,y+2,SEC,14); f.t(x+36,y,name,13,TEXT2,500)
        else:
            f.icon("file",x+16,y+2,FAINT if name!=active else ACCT,14)
            f.t(x+36,y,name,13,SEC if name!=active else TEXT,400 if name!=active else 500)
        if badge: f.pill(270,y+1,badge,ACCT,ACCS,10,18,6,9,600)
        y+=26
    if footer:
        f.line(64,804,296,804,SUBTLE)
        f.icon("calendar",72,818,SEC,15); f.t(94,816,"Today",13,TEXT2,500); f.t(140,817,"Thu, Nov 13",12,TER)
        f.icon("upload",72,850,SEC,15); f.t(94,848,"Import Obsidian vault",13,TEXT2,500)
def tabs(f, names, active, x0=MX, w=MW):
    f.rect(x0,0,w,40,fill=PANEL,name="Tab bar"); f.line(x0,40,x0+w,40,SUBTLE)
    x=x0+8
    for n in names:
        lab=n.replace("Query: ","")
        tw=min(240,int(len(lab)*7.4)+60)
        if n==active:
            f.rect(x,6,tw,34,fill=BG,r=8); f.line(x,40,x+tw,40,BG,2)
        f.icon("table" if n.startswith("Query") else ("graph" if n=="Graph view" else "file"),x+10,15,ACCT if n==active else FAINT,14)
        f.t(x+30,12,lab,12,TEXT if n==active else TER,500,w=tw-46)
        f.icon("x",x+tw-22,16,FAINT if n!=active else SEC,12)
        x+=tw+4
    f.icon("plus",x+6,14,TER,14)
    f.icon("sidebar",x0+w-30,13,TER,15)
def pageheader(f, crumbs, audience="Private", persona="Elminster edits directly", x0=MX, w=MW):
    x=x0+24
    for i,c in enumerate(crumbs):
        last=i==len(crumbs)-1
        f.t(x,54,c,12,TEXT2 if last else TER,500 if last else 400); x+=int(len(c)*7.2)+10
        if not last: f.t(x-4,54,"/",12,FAINT); x+=10
    rx=x0+w-24
    rx-=28; f.icon("more",rx+6,54,TER)
    rx-=28; f.icon("link",rx+6,54,TER)
    if persona:
        pw=int(len(persona)*6.6)+44; rx-=pw+8
        f.rect(rx,48,pw,26,fill=SURF,stroke=BORDER,r=13)
        f.dot(rx+6,53,16,"#3FC1B033"); f.t(rx+10,54,"E",10,TEAL,700,w=10)
        f.t(rx+28,53,persona,12,SEC,500,w=pw-30)
    if audience:
        icon={"Private":"lock","Shared":"share","Agent":"bot"}[audience]
        col={"Private":ACCT,"Shared":BLUE,"Agent":ORANGE}[audience]
        bg={"Private":ACCS,"Shared":BLUES,"Agent":ORANGES}[audience]
        aw=int(len(audience)*7)+50; rx-=aw+8
        f.rect(rx,48,aw,26,fill=bg,r=13); f.icon(icon,rx+10,53,col,14); f.t(rx+28,53,audience,12,col,600); f.icon("chev-d",rx+aw-20,55,col,11)
def statusbar(f, left, right, x0=MX):
    f.rect(x0,876,1440-x0,24,fill=PANEL); f.line(x0,876,1440,876,SUBTLE)
    f.t(x0+16,880,left,11,TER,w=500)
    f.t(1440-16-int(len(right)*6.4),880,right,11,TER,w=int(len(right)*6.4)+4)
def rightpanel(f, active="link", title=None):
    f.rect(RX,0,RW,876,fill=PANEL,name="Right sidebar"); f.line(RX,0,RX,876,SUBTLE)
    f.line(RX,40,1440,40,SUBTLE)
    x=RX+14
    for ic in ["link","list","graph","chat","clock"]:
        if ic==active: f.rect(x-6,8,28,26,fill=SEL,r=6)
        f.icon(ic,x,13,TEXT if ic==active else TER,15); x+=38
    if title: f.t(RX+18,56,title,11,TER,600,letter=None) if False else f.t(RX+18,56,title.upper(),10,TER,700)

def explorer_hl(f, active, tree=None):
    y=128
    for lvl,kind,name,badge in (tree or DEFAULT_TREE):
        if name==active:
            f.rect(60,y-3,240,26,fill=SEL,r=6); f.rect(60,y+2,2,16,fill=ACC,r=1)
        y+=26
