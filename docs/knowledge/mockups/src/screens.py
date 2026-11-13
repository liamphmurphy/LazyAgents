from shell import *
EX,EW=374,660  # editor column
def L(t): return (t,15,ACC,500)          # wikilink run
def U(t): return (t,15,"#6E66A8",400,{"underline":True})  # unresolved link
def P(t,c=TEXT2): return (t,15,c)
def props(f,y,rows):
    f.t(EX,y,"Properties",12,TER,600); y+=24
    for ic,k,vals in rows:
        f.icon(ic,EX,y+5,FAINT,14); f.t(EX+22,y+3,k,13,TER,w=110)
        x=EX+140
        for v in vals:
            if v.startswith("#"): x+=f.pill(x,y+3,v,ACCT,ACCS,12,22,9,11,500)+6
            elif v.startswith("[["): f.t(x,y+3,v[2:-2],13,ACC,500); x+=int(len(v)*7.5)
            else: f.t(x,y+3,v,13,TEXT2); x+=int(len(v)*7.5)+8
        y+=30
    f.icon("plus",EX,y+5,FAINT,13); f.t(EX+22,y+3,"Add property",12,FAINT); y+=34
    f.line(EX,y,EX+EW,y,SUBTLE)
    return y+18
def mapcard(f,x,y,w,h):
    f.rect(x,y,w,h,fill="#122033",r=10,stroke=BORDER)
    f.add({"type":"path","name":"coast","x":x,"y":y,"w":w,"h":h,"d":f"M 0 {h*0.55} C {w*0.2} {h*0.45} {w*0.35} {h*0.7} {w*0.55} {h*0.6} C {w*0.75} {h*0.5} {w*0.85} {h*0.3} {w} {h*0.35} L {w} 0 L 0 0 Z","fill":"#2A2A22","stroke":"#4B4A3A","stroke_width":1.5})
    for i,px in enumerate([0.3,0.45,0.62]):
        f.rect(x+w*px,y+h*0.52+i*4,8,h*0.28,fill="#6B5B45",r=2)
    f.dot(x+w*0.72-6,y+h*0.32,12,STAR); f.t(x+w*0.72+10,y+h*0.29,"Watchtower",11,"#E8DCC8",600)
    f.t(x+w*0.5,y+h*0.86,"North pier",11,"#C8D8E8",500)
    f.rect(x+10,y+h-30,96,20,fill="#0C0B12CC",r=6); f.icon("file",x+16,y+h-27,SEC,13); f.t(x+34,y+h-28,"harbour.png",11,TEXT2,500)
def mini_graph(f,cx,cy,labels):
    import math
    n=len(labels)
    pts=[(cx+math.cos(i/n*6.283-1.4)*92,cy+math.sin(i/n*6.283-1.4)*62) for i in range(n)]
    for (x,y) in pts: f.line(cx,cy,x,y,"#3A3650",1.2)
    f.line(pts[0][0],pts[0][1],pts[1][0],pts[1][1],"#2A2738",1)
    for (x,y),lab in zip(pts,labels):
        unres=lab.startswith("?")
        f.dot(x-5,y-5,10,"#2A2738" if unres else "#6E64B8",stroke=FAINT if unres else None)
        f.t(x-55,y+8,lab.lstrip("?"),10,TER,w=110,align="center")
    f.dot(cx-8,cy-8,16,ACC); f.t(cx-50,cy+10,"The old harbour",10,TEXT,600,w=100,align="center")

def editor_body(f, y, mode="plain"):
    f.text(EX,y,EW,[[P("The ferry runs twice a day, at dawn and dusk. "),L("Mara Vell"),P(" keeps the harbour master's log in the "),L("Watchtower"),P(".")]],h=50,ls=1.25); y+=62
    f.t(EX,y,"Arrival",20,TEXT,600); y+=38
    f.text(EX,y,EW,[[P("Travellers come in at the north pier, where the tide bell rings once per crossing. "),("^arrival",13,FAINT)]],h=50,ls=1.25); y+=62
    # callout
    f.rect(EX,y,EW,74,fill="#9B8CFF12",r=8); f.rect(EX,y,3,74,fill=ACC,r=1)
    f.icon("info",EX+16,y+12,ACCT,15); f.t(EX+38,y+11,"Secret",14,ACCT,600)
    f.text(EX+16,y+38,EW-32,[[P("The bell tower hides a cache. Only "),L("Mara Vell"),P(" knows the way into "),U("Smuggler tunnels"),P(".")]],h=26); y+=90
    return y

def s_editor(x,y):
    f=F("1 · Editor: live preview, links, local graph",x,y)
    rail(f); explorer(f); tabs(f,["The old harbour","Session 4","Mara Vell"],"The old harbour")
    pageheader(f,["Campaign","Locations","The old harbour"])
    f.t(EX,98,"The old harbour",30,TEXT,700,w=EW)
    yy=props(f,148,[("tag","type",["location"]),("tag","tags",["#campaign","#saltmere"]),("link","aliases",["Saltmere docks"]),("link","region",["[[Saltmere]]"])])
    yy=editor_body(f,yy)
    # embed image + embedded block side by side
    mapcard(f,EX,yy,320,170)
    bx=EX+336
    f.rect(bx,yy,EW-336,170,fill=SURF,r=10,stroke=BORDER); f.rect(bx,yy,3,170,fill=STRONG)
    f.icon("link",bx+14,yy+12,TER,13); f.t(bx+32,yy+10,"Session 4  ›  ^dawn-landing",11,TER,500)
    f.text(bx+14,yy+36,EW-336-28,[[("We tied up at ",14,TEXT2),("The old harbour",14,ACC,500),(" before dawn. The bell rang twice, which never happens.",14,TEXT2)]],h=80,ls=1.25)
    f.t(bx+14,yy+140,"Embedded block · updates live",11,FAINT)
    yy+=186
    f.icon("check",EX+2,yy+3,GREEN,14); f.rect(EX,yy+1,18,18,stroke=GREEN,r=4); f.t(EX+28,yy,"Name the ferry captain",15,TER,w=300,strike=True)
    f.rect(EX,yy+31,18,18,stroke=STRONG,r=4); f.t(EX+28,yy+30,"Map the smuggler tunnels",15,TEXT2,w=300)
    rightpanel(f,"link")
    f.t(RX+18,56,"LINKED MENTIONS",10,TER,700); f.pill(RX+128,53,"4",SEC,SEL,10,18,6,9,600)
    cy=82
    for title,pre,post in [("Session 4","…we tied up at "," before dawn…"),("Mara Vell","Harbour master of ",". Keeps the log."),("Ferry schedule","Departs "," at 06:00 and 18:00.")]:
        f.rect(RX+12,cy,316,62,fill=SURF,r=8,stroke=SUBTLE)
        f.icon("file",RX+24,cy+10,FAINT,13); f.t(RX+42,cy+8,title,13,TEXT,600)
        f.text(RX+24,cy+32,292,[[(pre,12,SEC),("The old harbour",12,TEXT,600,{"highlight":"#9B8CFF33"}),(post,12,SEC)]],h=22)
        cy+=70
    f.t(RX+18,cy+8,"UNLINKED MENTIONS",10,TER,700); f.pill(RX+140,cy+5,"2",SEC,SEL,10,18,6,9,600); cy+=34
    for title,pre,post in [("Session 7","…back to "," for supplies."),("Rumours","Sailors say "," is cursed.")]:
        f.rect(RX+12,cy,316,62,fill=SURF,r=8,stroke=SUBTLE)
        f.icon("file",RX+24,cy+10,FAINT,13); f.t(RX+42,cy+8,title,13,TEXT,600)
        f.text(RX+24,cy+32,230,[[(pre,12,SEC),("the old harbour",12,TEXT2,600,{"underline":True}),(post,12,SEC)]],h=22)
        f.button(RX+266,cy+18,"Link",h=26,size=12,w=50)
        cy+=70
    f.line(RX,cy+6,1440,cy+6,SUBTLE)
    f.t(RX+18,cy+20,"LOCAL GRAPH",10,TER,700); f.t(RX+250,cy+20,"depth 1 ▾",10,TER,500)
    mini_graph(f,RX+170,cy+130,["Session 4","Mara Vell","Ferry schedule","Watchtower","Saltmere","?Smuggler tunnels"])
    statusbar(f,"Elminster edits this page directly · you review after","4 backlinks    186 words    Saved")
    return f

def s_chat(x,y):
    f=F("2 · Page chat: a persona edits directly",x,y)
    rail(f); explorer(f); tabs(f,["The old harbour"],"The old harbour")
    pageheader(f,["Campaign","Locations","The old harbour"])
    f.t(EX,98,"The old harbour",30,TEXT,700,w=EW)
    f.t(EX,146,"location  ·  #campaign  #saltmere  ·  region Saltmere",12,TER)
    yy=184
    yy=editor_body(f,yy)
    # new section added by persona, marked in gutter
    f.rect(EX-20,yy-4,3,236,fill=TEAL,r=1)
    f.rect(EX+EW-130,yy+2,130,24,fill="#3FC1B01F",r=12); f.t(EX+EW-130,yy+6,"Added by Elminster",11,TEAL,600,w=130,align="center")
    f.t(EX,yy,"History",20,TEXT,600); yy+=38
    f.text(EX,yy,EW,[[("Built by the Saltmere guild after the flood of 1102. ",15,TEXT2,400,{"highlight":"#3FC1B01A"}),("Captain Oren",15,ACC,500,{"highlight":"#3FC1B01A"}),(" ran the first ferry; his granddaughter ",15,TEXT2,400,{"highlight":"#3FC1B01A"}),("Mara Vell",15,ACC,500,{"highlight":"#3FC1B01A"}),(" now keeps the log.",15,TEXT2,400,{"highlight":"#3FC1B01A"})]],h=76,ls=1.3); yy+=96
    f.rect(EX,yy,EW,64,fill=SURF,r=8,stroke=BORDER)
    f.icon("sparkle",EX+14,yy+14,TEAL,15); f.t(EX+38,yy+12,"Elminster changed this page · 2 changes",13,TEXT,600)
    f.t(EX+38,yy+34,"Applied directly, as this page allows. Undo or review in History.",12,TER,w=440)
    f.button(EX+EW-176,yy+17,"Undo",h=28,size=12,w=64,icon=None); f.button(EX+EW-104,yy+17,"Review",h=28,size=12,w=88)
    rightpanel(f,"chat")
    f.dot(RX+16,52,28,"#3FC1B033"); f.t(RX+25,57,"E",13,TEAL,700,w=12)
    f.t(RX+52,50,"Elminster",14,TEXT,600); f.t(RX+52,69,"Chat about this page",11,TER)
    f.line(RX,92,1440,92,SUBTLE)
    # user bubble
    f.rect(RX+70,112,256,62,fill=SEL,r=12)
    f.text(RX+84,122,232,[[("Add a short history for the harbour and link the NPCs we already have.",13,TEXT)]],h=44)
    # persona reply
    f.dot(RX+14,192,22,"#3FC1B033"); f.t(RX+21,195,"E",11,TEAL,700,w=10)
    f.rect(RX+44,190,282,34,fill=SURF,r=8,stroke=SUBTLE); f.icon("search",RX+54,199,TER,14); f.t(RX+74,198,"search_knowledge  ·  #npc  ·  3 pages",12,SEC,w=240)
    f.rect(RX+44,232,282,34,fill=SURF,r=8,stroke=SUBTLE); f.icon("pen",RX+54,241,TEAL,14); f.t(RX+74,240,"patch_page  ·  2 changes",12,SEC,w=246)
    f.text(RX+44,278,282,[[("Added a ",14,TEXT2),("History",14,TEXT,600),(" section and linked ",14,TEXT2),("Captain Oren",14,ACC,500),(" and ",14,TEXT2),("Mara Vell",14,ACC,500),(". You let me edit this page directly, so it's live; both changes are in History if you want them back.",14,TEXT2)]],h=110,ls=1.3)
    f.text(RX+44,394,282,[[("I didn't find a page for the flood of 1102. Want me to start one from the ",14,TEXT2),("Location",14,TEXT,600),(" template?",14,TEXT2)]],h=60,ls=1.3)
    f.button(RX+44,462,"Create it",h=28,size=12,w=90); f.button(RX+142,462,"Not now",h=28,size=12,w=82)
    f.rect(RX+12,784,316,80,fill=SURF,r=12,stroke=STRONG)
    f.t(RX+26,798,"Ask Elminster about this page…",13,TER)
    f.icon("link",RX+26,836,TER,14); f.icon("plus",RX+50,836,TER,14)
    f.rect(RX+290,828,28,28,fill=ACC,r=8); f.icon("upload",RX+296,834,"#120F24",16)
    statusbar(f,"Elminster edits this page directly · you review after","6 backlinks    242 words    Saved")
    return f

def s_review(x,y):
    f=F("3 · Review persona suggestions change by change",x,y)
    rail(f); explorer(f,active="Watchtower"); tabs(f,["Watchtower","The old harbour"],"Watchtower")
    pageheader(f,["Campaign","Locations","Watchtower"],persona="Elminster suggests changes")
    # review bar
    f.rect(MX+16,86,MW-32,48,fill="#F59E5B14",r=10,stroke="#F59E5B40")
    f.icon("sparkle",MX+32,102,ORANGE,15)
    f.text(MX+56,99,400,[[("Elminster suggested 3 changes",13,TEXT,600),("   1 is stale",13,ORANGE,500)]],h=22)
    f.button(MX+MW-262,95,"Reject rest",h=30,size=12,w=104); f.button(MX+MW-150,95,"Accept rest",primary=True,h=30,size=12,w=118)
    f.t(EX,156,"Watchtower",30,TEXT,700)
    f.t(EX,202,"location  ·  #campaign  #saltmere  ·  region Saltmere",12,TER)
    yy=240
    # change 1: replace
    f.text(EX,yy,EW-90,[[("The tower stands at the end of the breakwater. The bell ",15,TEXT2),("rings at dawn",15,"#F48A87",400,{"strike":True,"highlight":REDS}),(" ",15,TEXT2),("rings once per ferry crossing, never at night",15,"#7BD3A6",400,{"highlight":GREENS}),(".",15,TEXT2)]],h=52,ls=1.3)
    for i,(ic,c) in enumerate([("check",GREEN),("x",RED)]):
        f.rect(EX+EW-74+i*38,yy,30,28,fill=SURF,r=7,stroke=STRONG); f.icon(ic,EX+EW-67+i*38,yy+6,c,15)
    f.t(EX,yy+56,"1 · Why: matches the bell in The old harbour and Session 4",12,TER,italic=True)
    yy+=96
    # change 2: insert
    f.t(EX,yy,"Keepers",20,TEXT,600); yy+=38
    f.text(EX,yy,EW-90,[[("Mara Vell has kept the log since the flood. ",15,TEXT2),("Before her, ",15,"#7BD3A6",400,{"highlight":GREENS}),("Captain Oren",15,ACC,500,{"highlight":GREENS}),(" kept it for forty years.",15,"#7BD3A6",400,{"highlight":GREENS})]],h=52,ls=1.3)
    for i,(ic,c) in enumerate([("check",GREEN),("x",RED)]):
        f.rect(EX+EW-74+i*38,yy,30,28,fill=SURF,r=7,stroke=STRONG); f.icon(ic,EX+EW-67+i*38,yy+6,c,15)
    f.t(EX,yy+56,"2 · Why: link the previous keeper",12,TER,italic=True)
    # rejecting with reason popover
    f.rect(EX+330,yy+36,330,116,fill=RAISED,r=10,stroke=STRONG)
    f.t(EX+346,yy+50,"Reject with a reason",12,TEXT,600)
    f.rect(EX+346,yy+74,298,32,fill=SUNK,r=7,stroke=ACC); f.t(EX+356,yy+81,"Oren never kept the log, he ran the ferry|",12,TEXT2,w=284)
    f.t(EX+346,yy+118,"Sent to Elminster in this page's chat",11,TER); f.button(EX+584,yy+114,"Reject",h=26,size=12,w=62)
    yy+=170
    # change 3 stale
    f.rect(EX,yy,EW,86,fill=AMBERS,r=10,stroke="#F0BF4C40")
    f.icon("alert",EX+16,yy+14,AMBER,15); f.t(EX+40,yy+12,"3 · Stale: you rewrote this passage after Elminster suggested it",13,TEXT,600,w=560)
    f.text(EX+40,yy+36,EW-60,[[("Your text was kept. Elminster gets the change back with this note so it can try again against your version.",13,SEC)]],h=40)
    yy+=104
    f.text(EX,yy,EW,[[("From the top you can see ",15,TEXT2),("Saltmere",15,ACC,500),(" and the whole of the north pier.",15,TEXT2)]],h=26)
    rightpanel(f,"clock")
    f.t(RX+18,56,"HISTORY",10,TER,700)
    f.rect(RX+12,80,316,72,fill=SURF,r=10,stroke=SUBTLE)
    f.t(RX+26,90,"Persona edits on this page",12,TEXT,600)
    segs=[("Direct",False),("Suggest",True),("Paused",False)]
    sx=RX+26
    f.rect(sx,116,288,26,fill=SUNK,r=7,stroke=SUBTLE)
    for i,(n,on) in enumerate(segs):
        if on: f.rect(sx+2+i*96,118,92,22,fill=SEL,r=6)
        f.t(sx+2+i*96,121,n,12,TEXT if on else TER,500,w=92,align="center")
    hy=170
    for who,col,what,when,cur in [("Elminster","#3FC1B033","Suggested 3 changes","2 min ago",False),("You","#3B3260","Rewrote the opening","6 min ago",True),("Elminster","#3FC1B033","Linked Saltmere · applied","Yesterday",False),("You","#3B3260","Created from Location template","Nov 2",False)]:
        f.dot(RX+18,hy+2,24,col); f.t(RX+26,hy+6,who[0],11,TEAL if who!="You" else TEXT,700,w=10)
        f.t(RX+52,hy,what,13,TEXT,500,w=210); f.t(RX+52,hy+20,f"{who} · {when}",11,TER,w=200)
        if cur: f.pill(RX+262,hy+2,"Current",GREEN,"#4CB7821F",10,20,7,10,600)
        else: f.t(RX+270,hy+4,"Restore",12,ACCT,500)
        f.line(RX+18,hy+48,RX+322,hy+48,SUBTLE); hy+=60
    statusbar(f,"Elminster suggests changes on this page · nothing changes until you accept","3 pending    Saved")
    return f

def s_graph(x,y):
    import random, math
    random.seed(7)
    f=F("4 · Graph view",x,y)
    rail(f,"book"); explorer(f,active=None); tabs(f,["The old harbour","Graph view"],"Graph view",w=1440-MX)
    f.rect(MX,41,1440-MX,835,fill="#0A0910")
    groups=[(ACC,"Campaign",(680,400),16),(TEAL,"Literature",(960,250),9),(AMBER,"Daily",(520,680),8),(BLUE,"Projects",(1000,660),7)]
    nodes=[]
    for col,_,(gx,gy),n in groups:
        for i in range(n):
            a=random.random()*6.283; r=random.random()**0.6*150
            nodes.append((gx+math.cos(a)*r,gy+math.sin(a)*r*0.8,col,random.choice([6,7,8,10,12])))
    hub=(680,400)
    for i,(nx,ny,c,s) in enumerate(nodes):
        near=sorted([(abs(nx-m[0])+abs(ny-m[1]),j) for j,m in enumerate(nodes) if j!=i and m[2]==c])
        for _,j in near[:1+(i%2)]: f.line(nx,ny,nodes[j][0],nodes[j][1],"#2A2740",1)
    for k in [16,20,27,30]: f.line(nodes[k][0],nodes[k][1],hub[0],hub[1],"#2A2740",1)
    # orphans
    for ox,oy in [(420,200),(1240,180),(480,520)]: nodes.append((ox,oy,FAINT,6))
    for nx,ny,c,s in nodes: f.dot(nx-s/2,ny-s/2,s,c)
    # highlighted hub
    for nx,ny,c,s in nodes[:6]: f.line(hub[0],hub[1],nx,ny,ACC,1.5)
    f.dot(hub[0]-11,hub[1]-11,22,ACC,stroke="#FFFFFF",sw=2)
    f.rect(hub[0]-62,hub[1]+16,124,22,fill="#0A0910CC",r=6); f.t(hub[0]-62,hub[1]+18,"The old harbour",12,TEXT,600,w=124,align="center")
    for (lx,ly,lab) in [(960,250,"How to take smart notes"),(520,680,"2026-11-13"),(1000,660,"Vesper roadmap")]:
        f.t(lx-80,ly+12,lab,11,SEC,w=160,align="center")
    # filters panel
    px,py=1440-300,60
    f.rect(px,py,280,430,fill=RAISED,r=12,stroke=STRONG)
    f.icon("filter",px+16,py+16,SEC,15); f.t(px+38,py+14,"Filters",13,TEXT,600)
    f.rect(px+16,py+44,248,30,fill=SUNK,r=7,stroke=SUBTLE); f.icon("search",px+26,py+51,TER,14); f.t(px+46,py+50,"Search files…",12,TER)
    yy=py+88
    for lab,on in [("Tags",False),("Attachments",False),("Orphans",True),("Agent knowledge",False)]:
        f.t(px+16,yy,lab,13,TEXT2); f.rect(px+226,yy,36,20,fill=ACC if on else STRONG,r=10); f.dot(px+(244 if on else 228),yy+2,16,"#FFFFFF" if on else SEC); yy+=32
    f.line(px+16,yy,px+264,yy,SUBTLE); yy+=14
    f.t(px+16,yy,"Groups",13,TEXT,600); yy+=26
    for col,lab in [(ACC,"path:Campaign"),(TEAL,"tag:#literature"),(AMBER,"path:Daily"),(BLUE,"path:Projects")]:
        f.dot(px+18,yy+3,12,col); f.t(px+38,yy,lab,12,TEXT2); yy+=26
    f.t(px+16,yy+4,"+ New group",12,ACCT,500); yy+=34
    f.line(px+16,yy,px+264,yy,SUBTLE); yy+=14
    f.t(px+16,yy,"Display  ›",13,TEXT2,500); f.t(px+140,yy,"Forces  ›",13,TEXT2,500)
    f.text(MX+24,820,600,[[("Agent knowledge is left out unless you turn it on. Private pages never show in a run's graph.",12,TER)]],h=20)
    statusbar(f,"1,312 pages · 9,876 links · 9 orphans","Graph of Your knowledge")
    return f

def s_switcher(x,y):
    f=F("5 · Quick switcher: go to a page or create it",x,y)
    base_editor(f)
    mx,my,mw=400,110,640
    f.rect(mx,my,mw,452,fill=RAISED,r=14,stroke=STRONG,name="Quick switcher")
    f.icon("search",mx+18,my+18,SEC,18); f.t(mx+48,my+15,"harb",17,TEXT,500); f.rect(mx+88,my+17,2,20,fill=ACC)
    f.kbd(mx+mw-56,my+18,"esc")
    f.line(mx,my+56,mx+mw,my+56,BORDER)
    rows=[("file","The old harbour","Campaign / Locations",True,"Private"),("file","Harbour master's log","Campaign / NPCs / Mara Vell › Log",False,"Private"),
          ("link","Harbourmaster","alias of Mara Vell",False,"Private"),("file","Harbour deploy runbook","Projects / Vesper",False,"Shared"),
          ("book","Harbors and the hinterland","Literature · from a saved link",False,"Private")]
    ry=my+66
    for ic,title,path,sel,aud in rows:
        if sel: f.rect(mx+8,ry,mw-16,52,fill=SEL,r=8)
        f.icon(ic,mx+22,ry+10,ACCT if sel else TER,15)
        hl=title.lower().find("harb")
        runs=[]
        if hl>=0:
            if hl: runs.append((title[:hl],14,TEXT))
            runs.append((title[hl:hl+4],14,ACCT,700,{"underline":True})); runs.append((title[hl+4:],14,TEXT))
        else: runs=[(title,14,TEXT)]
        f.text(mx+48,ry+7,380,[runs],h=22); f.t(mx+48,ry+29,path,12,TER,w=420)
        f.icon("lock" if aud=="Private" else "share",mx+mw-40,ry+18,FAINT if aud=="Private" else BLUE,14)
        if sel: f.kbd(mx+mw-82,ry+17,"↵")
        ry+=56
    f.rect(mx+8,ry,mw-16,44,fill=None,r=8,stroke=BORDER,dash="dash")
    f.icon("plus",mx+22,ry+14,ACCT,15); f.text(mx+48,ry+12,400,[[("Create  ",14,TEXT2),("“harb”",14,TEXT,600),("  in Inbox",14,TER)]],h=22); f.kbd(mx+mw-60,ry+13,"⇧↵")
    fy=my+452-36
    f.line(mx,fy,mx+mw,fy,BORDER)
    hx=mx+18
    for k,lab in [("↑↓","navigate"),("↵","open"),("⌘↵","new tab"),("⇧↵","create"),("#","search tags")]:
        hx+=f.kbd(hx,fy+9,k)+6; f.t(hx,fy+9,lab,11,TER); hx+=int(len(lab)*6.5)+18
    # capture
    cx,cy=400,600
    f.rect(cx,cy,mw,150,fill=RAISED,r=14,stroke=STRONG,name="Quick capture")
    f.icon("inbox",cx+18,cy+18,TEAL,16); f.t(cx+44,cy+16,"Quick capture",14,TEXT,600); f.t(cx+164,cy+18,"⌥⌘N from anywhere in Vesper",12,TER,w=260)
    f.rect(cx+16,cy+48,mw-32,52,fill=SUNK,r=8,stroke=SUBTLE)
    f.text(cx+28,cy+56,mw-60,[[("Ask ",14,TEXT2),("Mara Vell",14,ACC,500),(" why the bell rang twice ",14,TEXT2),("#todo",13,ACCT,500,{"highlight":ACCS})]],h=24)
    f.t(cx+16,cy+114,"Goes to Inbox. File it later, or let a persona file it for you.",12,TER)
    f.button(cx+mw-108,cy+108,"Capture",primary=True,h=28,size=12,w=92)
    return f

def s_daily(x,y):
    f=F("6 · Daily note, calendar and inbox",x,y)
    rail(f,"book"); explorer(f,active="2026-11-13"); tabs(f,["2026-11-13","The old harbour"],"2026-11-13")
    pageheader(f,["Daily","2026-11-13"],persona="Elminster edits directly")
    f.t(EX,98,"Thursday, 13 November",30,TEXT,700,w=EW)
    f.text(EX,146,EW,[[("type daily  ·  from template ",12,TER),("Daily",12,ACC,500),("  ·  ",12,TER),("← Nov 12",12,ACCT,500),("    ",12,TER),("Nov 14 →",12,FAINT,500)]],h=20)
    yy=186
    f.t(EX,yy,"Notes",20,TEXT,600); yy+=36
    for runs in [[P("Prepped "),L("Session 12"),P(": open at "),L("The old harbour"),P(" at dawn.")],[P("Read "),L("How to take smart notes"),P(" ch. 3; atomic notes stick.")],[P("Vesper: Knowledge PR is up for review "),("#vesper",13,ACCT,500,{"highlight":ACCS})]]:
        f.dot(EX+6,yy+9,5,SEC); f.text(EX+22,yy,EW-22,[runs],h=24); yy+=32
    yy+=12; f.t(EX,yy,"Tasks",20,TEXT,600); yy+=36
    for done,txt in [(True,"Email the group the session time"),(False,"Write up Mara Vell's motives"),(False,"Review Elminster's 3 suggestions on Watchtower")]:
        f.rect(EX,yy+1,18,18,stroke=GREEN if done else STRONG,r=4)
        if done: f.icon("check",EX+2,yy+3,GREEN,14)
        f.t(EX+28,yy,txt,15,TER if done else TEXT2,w=500,**({"strike":True} if done else {})); yy+=32
    yy+=12; f.t(EX,yy,"Captured today",20,TEXT,600); yy+=36
    for t in ["Ask Mara Vell why the bell rang twice","Idea: a ferry strike during the festival"]:
        f.icon("inbox",EX,yy+3,TEAL,14); f.t(EX+24,yy,t,15,TEXT2,w=500); yy+=30
    rightpanel(f,"calendar" if False else "clock")
    f.t(RX+18,56,"NOVEMBER 2026",10,TER,700); f.icon("chev-r",RX+300,54,TER,14); f.icon("chev-r",RX+280,54,FAINT,14)
    days="MTWTFSS"
    for i,d in enumerate(days): f.t(RX+22+i*44,84,d,11,FAINT,600,w=20,align="center")
    for day in range(1,31):
        idx=day+5; r,c=idx//7,idx%7
        cxp,cyp=RX+16+c*44,106+r*38
        if day==13: f.rect(cxp,cyp,32,30,fill=ACC,r=8)
        f.t(cxp,cyp+6,str(day),13,"#120F24" if day==13 else (TEXT2 if day<=13 else FAINT),600 if day==13 else 400,w=32,align="center")
        if day in (2,3,5,6,9,10,11,12): f.dot(cxp+14,cyp+25,4,ACC)
    f.line(RX,350,1440,350,SUBTLE)
    f.t(RX+18,366,"INBOX",10,TER,700); f.pill(RX+64,363,"3",SEC,SEL,10,18,6,9,600)
    f.rect(RX+12,392,316,36,fill=SUNK,r=8,stroke=SUBTLE); f.t(RX+24,400,"Capture a thought…",13,TER); f.kbd(RX+296,401,"↵")
    iy=440
    for t,src in [("Ask Mara Vell why the bell rang twice","Today 09:12 · quick capture"),("Ferry strike during the festival","Today 11:40 · from chat with Elminster"),("Read: Zettelkasten in practice","Yesterday · saved link")]:
        f.rect(RX+12,iy,316,92,fill=SURF,r=10,stroke=SUBTLE)
        f.t(RX+26,iy+12,t,13,TEXT,500,w=290); f.t(RX+26,iy+34,src,11,TER,w=290)
        f.button(RX+26,iy+56,"File into…",h=26,size=12,w=94,icon=None); f.button(RX+128,iy+56,"Merge into…",h=26,size=12,w=104)
        iy+=102
    f.t(RX+18,iy+6,"Elminster can file these for you",12,ACCT,500)
    statusbar(f,"Created from the Daily template on first open","2 backlinks    96 words    Saved")
    return f

def s_query(x,y):
    f=F("7 · Saved query as a table",x,y)
    rail(f); explorer(f,active="Campaign locations"); tabs(f,["Query: Campaign locations","The old harbour"],"Query: Campaign locations",w=1440-MX)
    X=MX+32; W=1440-MX-64
    f.icon("table",X,64,ACCT,20); f.t(X+30,58,"Campaign locations",26,TEXT,700)
    f.pill(X+310,64,"Saved query",ACCT,ACCS,11,22,9,11,600)
    f.button(1440-32-228,60,"Copy embed",h=30,size=12,w=110,icon="link"); f.button(1440-32-110,60,"New page",primary=True,h=30,size=12,w=110,icon="plus")
    cx=X; cy=110
    for chip in ["path is Campaign/","type = location","updated after Oct 1"]:
        w=int(len(chip)*7)+40; f.rect(cx,cy,w,28,fill=SURF,r=14,stroke=STRONG); f.icon("filter",cx+10,cy+7,TER,13); f.t(cx+28,cy+5,chip,12,TEXT2,500); cx+=w+8
    f.rect(cx,cy,90,28,r=14,stroke=STRONG,dash="dash"); f.t(cx+14,cy+5,"+ Filter",12,TER,500)
    f.t(1440-32-420,cy+5,"Group by  region ▾",12,SEC,500); f.t(1440-32-282,cy+5,"Sort  updated ↓",12,SEC,500)
    sx=1440-32-150
    f.rect(sx,cy,150,28,fill=SUNK,r=7,stroke=SUBTLE)
    for i,(n,on) in enumerate([("Table",True),("List",False),("Cards",False)]):
        if on: f.rect(sx+2,cy+2,48,24,fill=SEL,r=6)
        f.t(sx+2+i*49,cy+5,n,12,TEXT if on else TER,500,w=48,align="center")
    cols=[("Name",260),("type",110),("region",140),("tags",210),("linked from",130),("audience",110),("updated",120)]
    ty=158
    f.rect(X,ty,W,36,fill=SUNK,r=0); f.line(X,ty+36,X+W,ty+36,BORDER)
    xx=X+14
    for n,w in cols: f.t(xx,ty+10,n,12,TER,600); xx+=w
    ry=ty+36
    data={"Saltmere":[("The old harbour","location","Saltmere",["#campaign","#saltmere"],"6","Private","2 min ago"),("Watchtower","location","Saltmere",["#campaign"],"4","Private","1 h ago"),("Ferry schedule","location","Saltmere",["#campaign","#ferry"],"3","Private","Yesterday"),("Smuggler tunnels","—","Saltmere",[],"2","—","not created")],
          "Greywood":[("Greywood crossing","location","Greywood",["#campaign"],"2","Private","Nov 8"),("The hollow oak","location","Greywood",["#campaign","#fey"],"1","Shared","Nov 3")]}
    for g,rows in data.items():
        f.rect(X,ry,W,32,fill=PANEL); f.icon("chev-d",X+12,ry+9,TER,13); f.t(X+32,ry+7,g,13,TEXT,600); f.t(X+32+len(g)*8+10,ry+8,str(len(rows)),12,TER); ry+=32
        for name,typ,reg,tags,lk,aud,upd in rows:
            f.line(X,ry+40,X+W,ry+40,"#181722")
            xx=X+14; missing=upd=="not created"
            f.icon("file",xx,ry+12,FAINT,14); f.t(xx+22,ry+10,name,14,"#6E66A8" if missing else ACC,500,**({"underline":True} if missing else {})); xx+=cols[0][1]
            f.t(xx,ry+11,typ,13,TEXT2); xx+=cols[1][1]
            f.t(xx,ry+11,reg,13,ACC if reg else TER,500); xx+=cols[2][1]
            tx=xx
            for tg in tags: tx+=f.pill(tx,ry+10,tg,ACCT,ACCS,11,20,7,10,500)+6
            xx+=cols[3][1]
            f.t(xx,ry+11,lk+(" page" if lk=="1" else " pages"),13,TEXT2); xx+=cols[4][1]
            if aud!="—": f.icon("lock" if aud=="Private" else "share",xx,ry+12,ACCT if aud=="Private" else BLUE,13); f.t(xx+20,ry+11,aud,13,TEXT2)
            else: f.t(xx,ry+11,"—",13,FAINT)
            xx+=cols[5][1]
            f.t(xx,ry+11,upd,13,ORANGE if missing else TER, italic=missing) if missing else f.t(xx,ry+11,upd,13,TER)
            ry+=40
    f.rect(X,ry+16,W,48,fill=SURF,r=10,stroke=SUBTLE)
    f.icon("info",X+16,ry+32,ACCT,15)
    f.text(X+42,ry+30,W-60,[[("Embed this table in any page with ",13,TEXT2),("![[Campaign locations]]",13,ACCT,500,{"highlight":ACCS}),(". It stays live, follows each viewer's audience, and personas query it with ",13,TEXT2),("list_pages",13,TEXT,600),(".",13,TEXT2)]],h=40)
    statusbar(f,"6 pages · 1 unresolved","Query saved in Campaign/Campaign locations")
    return f

def s_import(x,y):
    f=F("8 · Import an Obsidian vault: dry run",x,y)
    base_editor(f)
    mx,my,mw,mh=250,50,940,800
    f.rect(mx,my,mw,mh,fill=RAISED,r=16,stroke=STRONG,name="Import dialog")
    f.icon("upload",mx+24,my+24,ACCT,18); f.t(mx+52,my+20,"Import an Obsidian vault",18,TEXT,700); f.icon("x",mx+mw-40,my+24,TER,16)
    sx=mx+24; sy=my+62
    for i,(lab,state) in enumerate([("Upload","done"),("Review","now"),("Import","todo")]):
        c={"done":GREEN,"now":ACC,"todo":FAINT}[state]
        f.dot(sx,sy,22,c if state!="todo" else SUNK,stroke=c)
        if state=="done": f.icon("check",sx+4,sy+4,"#120F24",14)
        else: f.t(sx,sy+3,str(i+1),11,"#120F24" if state=="now" else TER,700,w=22,align="center")
        f.t(sx+30,sy+2,lab,13,TEXT if state!="todo" else TER,600 if state=="now" else 500)
        if i<2: f.line(sx+30+len(lab)*8+12,sy+11,sx+190,sy+11,BORDER)
        sx+=200
    f.text(mx+24,my+100,mw-48,[[("my-vault.zip",13,TEXT,600),("  ·  412 MB  ·  scanned in 18 s  ·  wrapping folder ",13,TER),("My vault/",13,TEXT2,500),(" removed  ·  nothing has been written yet",13,TER)]],h=22)
    tx=mx+24; ty=my+134
    for n,lab,col in [("1,284","notes",TEXT),("3,402","attachments",TEXT),("214","tags",TEXT),("9,876","links resolved",GREEN),("41","unresolved",ORANGE)]:
        f.rect(tx,ty,172,66,fill=SURF,r=10,stroke=SUBTLE); f.t(tx+14,ty+10,n,22,col,700); f.t(tx+14,ty+40,lab,12,TER); tx+=181
    ty+=84
    tabx=mx+24
    for lab,n,on in [("All","1,292",True),("Renamed","3",False),("Conflicts","2",False),("Unresolved","41",False),("Kept as files","6",False),("Skipped","1",False)]:
        w=int(len(lab)*7.9+len(n)*7)+40
        if on: f.rect(tabx,ty,w,28,fill=SEL,r=7)
        f.t(tabx+12,ty+5,lab,12,TEXT if on else SEC,600 if on else 500); f.t(tabx+12+int(len(lab)*7.9)+6,ty+6,n,11,TER); tabx+=w+6
    ly=ty+40
    f.rect(mx+24,ly,mw-48,312,fill=SUNK,r=10,stroke=SUBTLE)
    rows=[("check",GREEN,"Campaign/Locations/The old harbour.md","Imported/Campaign/Locations/The old harbour","14 links, 1 embed"),
          ("pen",ORANGE,"Journal/[draft] ideas.md","Imported/Journal/draft ideas","Renamed: [ ] not allowed · old name kept as an alias"),
          ("alert",AMBER,"Campaign/NPCs/Mara Vell.md","Mara Vell (already in Knowledge)","Conflict: edited in Knowledge since your last import"),
          ("link",ORANGE,"Sessions/Session 4.md","Imported/Sessions/Session 4","2 links unresolved: [[North gate]], [[Bell keeper]]"),
          ("file",BLUE,"Maps/Saltmere.canvas","kept as an attachment","Canvas files aren't converted; listed here"),
          ("table",BLUE,"Bases/Campaign.base","kept as an attachment","Bases aren't converted; use a saved query"),
          ("x",RED,"Journal/broken.md","skipped","Frontmatter isn't valid YAML (line 3)")]
    yy=ly+10
    for ic,col,src,dst,note in rows:
        f.icon(ic,mx+40,yy+10,col,15)
        f.t(mx+66,yy+2,src,13,TEXT,500,w=400); f.t(mx+66,yy+22,note,12,TER,w=440)
        f.icon("chev-r",mx+506,yy+10,FAINT,14); f.t(mx+528,yy+10,dst,12,SEC,w=360)
        if ic=="alert":
            ox=mx+mw-48-170
            f.rect(ox,yy+6,150,26,fill=SURF,r=7,stroke=STRONG)
            for i,(lab,on) in enumerate([("Skip",False),("Keep both",True)]):
                if on: f.rect(ox+52,yy+8,96,22,fill=SEL,r=6)
                f.t(ox+(4 if i==0 else 52),yy+11,lab,11,TEXT if on else TER,500,w=(46 if i==0 else 96),align="center")
        f.line(mx+36,yy+41,mx+mw-60,yy+41,"#181722"); yy+=42
    oy=ly+336
    f.t(mx+24,oy,"Import into",12,TER,600); f.rect(mx+24,oy+20,260,32,fill=SURF,r=7,stroke=STRONG); f.icon("folder",mx+34,oy+28,SEC,15); f.t(mx+56,oy+27,"Imported/",13,TEXT2)
    f.t(mx+304,oy,"When a page already exists",12,TER,600)
    sx=mx+304; f.rect(sx,oy+20,300,32,fill=SUNK,r=7,stroke=SUBTLE)
    for i,(n,on) in enumerate([("Skip",False),("Replace",False),("Keep both",True)]):
        if on: f.rect(sx+2+i*99,oy+22,96,28,fill=SEL,r=6)
        f.t(sx+2+i*99,oy+28,n,12,TEXT if on else TER,500,w=96,align="center")
    f.t(mx+624,oy,"Who can see them",12,TER,600); f.rect(mx+624,oy+20,292,32,fill=ACCS,r=7); f.icon("lock",mx+636,oy+28,ACCT,15); f.t(mx+658,oy+27,"Private: you and your personas",13,ACCT,500)
    f.line(mx,my+mh-64,mx+mw,my+mh-64,BORDER)
    f.icon("info",mx+24,my+mh-42,TER,15); f.t(mx+48,my+mh-44,"Nothing is written until you import. You can undo the whole import afterwards.",12,TER,w=520)
    f.button(mx+mw-300,my+mh-48,"Cancel",h=32,w=86); f.button(mx+mw-204,my+mh-48,"Import 1,284 notes",primary=True,h=32,w=180)
    return f

def s_import_states(x,y):
    f=F("9 · Vault import: upload, progress, done, refused",x,y)
    f.node["fill"]="#0A0910"
    f.t(40,32,"Vault import states",22,TEXT,700); f.t(40,64,"Shown in the import dialog and, while it runs, as a card in Needs you",13,TER)
    cards=[(40,110,"Upload"),(740,110,"Scanning and importing"),(40,500,"Done"),(740,500,"Refused archive")]
    for cx,cy,title in cards:
        f.rect(cx,cy,660,360,fill=RAISED,r=16,stroke=STRONG,name=title); f.t(cx+24,cy+20,title,15,TEXT,600)
    cx,cy=40,110
    f.rect(cx+24,cy+56,612,220,fill=SUNK,r=12,stroke="#9B8CFF66",dash="dash")
    f.dot(cx+306,cy+90,48,ACCS); f.icon("upload",cx+318,cy+102,ACCT,24)
    f.t(cx+24,cy+150,"Drop a zipped vault here",16,TEXT,600,w=612,align="center")
    f.text(cx+24,cy+176,612,[[("or ",13,TER),("choose a file",13,ACCT,600),("  ·  .zip up to 2 GB",13,TER)]],h=20,align="center")
    f.t(cx+24,cy+214,"Your .obsidian settings, .trash and plugin data are ignored",12,FAINT,w=612,align="center")
    f.t(cx+24,cy+296,"Import into",12,TER,600); f.rect(cx+24,cy+316,240,30,fill=SURF,r=7,stroke=STRONG); f.t(cx+36,cy+322,"Imported/",13,TEXT2)
    f.rect(cx+280,cy+316,240,30,fill=ACCS,r=7); f.icon("lock",cx+292,cy+323,ACCT,14); f.t(cx+312,cy+322,"Private by default",13,ACCT,500)
    cx,cy=740,110
    for i,(lab,pct,note,col) in enumerate([("Scanning my-vault.zip",1.0,"4,692 files checked · report ready",GREEN),("Importing",0.62,"798 of 1,284 notes · Campaign/Sessions/Session 9.md",ACC)]):
        yy=cy+64+i*110
        f.t(cx+24,yy,lab,13,TEXT,600); f.t(cx+560,yy,f"{int(pct*100)}%",13,col,700,w=76,align="right")
        f.rect(cx+24,yy+26,612,8,fill=STRONG,r=4); f.rect(cx+24,yy+26,max(8,612*pct),8,fill=col,r=4)
        f.t(cx+24,yy+46,note,12,TER,w=600)
    f.rect(cx+24,cy+284,612,52,fill=REDS,r=8); f.icon("x",cx+38,cy+302,RED,14)
    f.text(cx+62,cy+292,560,[[("1 file skipped so far: ",12,TEXT2,600),("Journal/broken.md, frontmatter isn't valid YAML. The rest keeps going.",12,SEC)]],h=36)
    cx,cy=40,500
    f.dot(cx+24,cy+60,40,"#4CB78233"); f.icon("check",cx+34,cy+70,GREEN,20)
    f.t(cx+80,cy+60,"Imported 1,283 notes and 3,402 attachments",16,TEXT,600); f.t(cx+80,cy+84,"Into Imported/ · 4 min · Nov 13, 10:42",12,TER)
    yy=cy+124
    for ic,col,txt in [("pen",ORANGE,"3 renamed, old names kept as aliases"),("alert",AMBER,"2 conflicts kept both: Mara Vell (1), Watchtower (1)"),("link",ORANGE,"41 unresolved links, listed in Keep notes connected"),("x",RED,"1 skipped: Journal/broken.md")]:
        f.icon(ic,cx+24,yy+2,col,15); f.t(cx+48,yy,txt,13,TEXT2,w=560); yy+=28
    f.button(cx+24,cy+300,"Open Imported/",primary=True,h=32,w=150,icon="folder"); f.button(cx+186,cy+300,"Undo this import",h=32,w=160,icon="undo")
    f.t(cx+360,cy+302,"Undo removes the pages it created",12,TER,w=280); f.t(cx+360,cy+320,"and restores the ones it replaced",12,TER,w=280)
    cx,cy=740,500
    f.dot(cx+24,cy+60,40,REDS); f.icon("alert",cx+36,cy+71,RED,18)
    f.t(cx+80,cy+60,"This archive can't be imported",16,TEXT,600); f.t(cx+80,cy+84,"my-vault.zip · nothing was written",12,TER)
    f.rect(cx+24,cy+124,612,92,fill=SUNK,r=10,stroke=SUBTLE)
    f.text(cx+40,cy+138,580,[[("It contains a path that leaves the vault: ",13,TEXT2),("../../etc/passwd",13,RED,600)],[("Archives with paths outside the vault, absolute paths, over 200,000 files or extreme compression are refused whole.",12,TER)]],h=64)
    f.button(cx+24,cy+300,"Choose another file",primary=True,h=32,w=180); f.button(cx+216,cy+300,"Cancel",h=32,w=86)
    return f

def s_agent(x,y):
    f=F("10 · Agent knowledge and page audience",x,y)
    tree=[(0,"folder-o","agents",None),(1,"folder-o","tech-lead",None),(2,"file","Memory",None),(2,"file","Partition gotchas",None),(1,"folder-c","atlas",None),(1,"folder-c","frontend-engineer",None),(0,"folder-o","Patterns",None),(1,"folder-c","Go",None),(1,"folder-c","React",None),(1,"bot","Proposed","2"),(0,"folder-c","ADRs",None)]
    rail(f); explorer(f,active="Memory",tree=tree,view="agent")
    tabs(f,["Memory"],"Memory"); pageheader(f,["agents","tech-lead","Memory"],audience="Agent",persona=None)
    f.rect(MX+16,86,MW-32,52,fill=ORANGES,r=10,stroke="#F59E5B40")
    f.icon("bot",MX+32,104,ORANGE,16)
    f.text(MX+58,96,MW-110,[[("Agent knowledge",13,TEXT,600),(" is written by agents for agents. It stays out of your search, graph, link suggestions and persona chats unless you open it here.",13,SEC)]],h=40)
    f.t(EX,160,"Memory",30,TEXT,700)
    f.t(EX,206,"agent tech-lead  ·  12 notes  ·  updated by runs",12,TER)
    yy=246
    for runs in [[P("Run "),("make gen",14,TEXT,600),P(" after editing any .sql query; "),L("Patterns/Go/sqlc"),P(" explains why.")],[P("Knowledge queries must call artifact_visible; see "),L("ADR-knowledge"),P(".")],[P("Playground needs VESPER_NO_SIGNIN=1; "),L("Partition gotchas"),P(" has the rest.")]]:
        f.dot(EX+6,yy+9,5,SEC); f.text(EX+22,yy,EW-22,[runs],h=24); yy+=34
    # audience popover anchored to chip
    px,py=MX+MW-24-300,80
    f.rect(px,py,300,282,fill=RAISED,r=12,stroke=STRONG,name="Audience menu")
    f.t(px+16,py+14,"Who can see this page",13,TEXT,600)
    oy=py+44
    for ic,col,bg,lab,desc,on in [("lock",ACCT,ACCS,"Private","You and your personas. Never reaches ticket or epic runs.",False),("share",BLUE,BLUES,"Shared","Also ticket and epic runs. Linked private pages stay private.",False),("bot",ORANGE,ORANGES,"Agent","Agent memory and proposed patterns. Hidden from your views.",True)]:
        if on: f.rect(px+8,oy-4,284,68,fill=SEL,r=8)
        f.rect(px+16,oy+2,28,28,fill=bg,r=8); f.icon(ic,px+22,oy+8,col,16)
        f.t(px+54,oy,lab,13,TEXT,600); f.text(px+54,oy+20,224,[[(desc,12,TER)]],h=36)
        if on: f.icon("check",px+268,oy+2,ACC,15)
        oy+=72
    f.t(px+16,py+258,"Only you can change this. Agents can't.",11,FAINT)
    rightpanel(f,"link"); f.t(RX+18,56,"LINKED MENTIONS",10,TER,700)
    cy=82
    for title,src in [("Run #812","LAZYAGENTS-181 · tech-lead"),("Run #790","LAZYAGENTS-176 · tech-lead"),("Patterns/Go/sqlc","shared page")]:
        f.rect(RX+12,cy,316,52,fill=SURF,r=8,stroke=SUBTLE); f.icon("bot" if title.startswith("Run") else "file",RX+24,cy+10,TER,13)
        f.t(RX+42,cy+8,title,13,TEXT,600); f.t(RX+42,cy+28,src,11,TER); cy+=60
    statusbar(f,"Agent page · written by runs · you can read and edit it","3 backlinks    Saved",)
    return f

def s_connect(x,y):
    f=F("11 · Keep notes connected: mentions, unresolved, orphans",x,y)
    rail(f); explorer(f,active=None); tabs(f,["Keep notes connected"],"Keep notes connected",w=1440-MX)
    X=MX+32; W=1440-MX-64
    f.icon("link",X,62,ACCT,20); f.t(X+30,56,"Keep notes connected",26,TEXT,700)
    f.rect(X,104,W,74,fill=SURF,r=12,stroke=BORDER)
    f.dot(X+16,120,40,"#3FC1B033"); f.t(X+29,130,"E",14,TEAL,700,w=14)
    f.text(X+70,118,640,[[("Elminster's weekly pass",14,TEXT,600),("  ·  Mondays 09:00",13,TER)],[("Last run found 7 links to add and 2 notes to split. They're waiting for you in Needs you.",13,SEC)]],h=44)
    f.button(X+W-270,126,"Review 9",primary=True,h=30,w=100); f.button(X+W-160,126,"Run now",h=30,w=86,icon=None); f.icon("settings",X+W-56,133,TER,16)
    tx=X; ty=200
    for lab,n,on in [("Unlinked mentions","12",True),("Unresolved links","41",False),("Orphans","9",False)]:
        w=int(len(lab)*7.6)+44
        f.t(tx,ty,lab,14,TEXT if on else TER,600 if on else 500); f.pill(tx+len(lab)*7.6+8,ty+1,n,SEC,SEL,10,18,6,9,600)
        if on: f.rect(tx,ty+28,w,2,fill=ACC)
        tx+=w+28
    f.line(X,ty+30,X+W,ty+30,SUBTLE)
    f.text(X,ty+46,W,[[("Where a page names another page without linking it. Link one, or select several and link them in one go.",13,TER)]],h=20)
    ly=ty+80; LW=600
    for page,pre,m,post,target in [("Session 7","…we went back to ","the old harbour"," for supplies…","The old harbour"),("Rumours","Sailors say ","the old harbour"," is cursed.","The old harbour"),("Session 9","…a letter from ","mara vell"," arrived…","Mara Vell"),("How to take smart notes","…write ","atomic notes"," so each idea…","Atomic notes"),("2026-11-12","Asked ","Elminster"," for a recap.","Elminster (persona)")]:
        f.rect(X,ly,LW,70,fill=SURF,r=10,stroke=SUBTLE)
        f.rect(X+14,ly+26,16,16,stroke=STRONG,r=4)
        f.icon("file",X+42,ly+12,FAINT,13); f.t(X+60,ly+10,page,13,TEXT,600)
        f.text(X+42,ly+34,440,[[(pre,13,SEC),(m,13,TEXT,600,{"highlight":"#9B8CFF33"}),(post,13,SEC),("  →  ",13,FAINT),(target,13,ACC,500)]],h=22)
        f.button(X+LW-72,ly+20,"Link",h=28,size=12,w=58); ly+=78
    RX2=X+LW+32; RW2=W-LW-32
    f.t(RX2,ty+80,"UNRESOLVED LINKS",10,TER,700)
    ry=ty+104
    for name,n in [("Smuggler tunnels",4),("North gate",3),("Bell keeper",2),("Flood of 1102",1)]:
        f.rect(RX2,ry,RW2,44,fill=SURF,r=8,stroke=SUBTLE)
        f.t(RX2+14,ry+12,name,13,"#6E66A8",500,underline=True); f.t(RX2+180,ry+13,f"{n} page"+("s" if n!=1 else ""),12,TER)
        f.t(RX2+RW2-104,ry+13,"Create page",12,ACCT,600); ry+=52
    f.t(RX2,ry+16,"ORPHANS",10,TER,700); ry+=40
    for name,where in [("Untitled 3","Inbox"),("Bell rope knots","Campaign/Lore"),("Meeting notes 2026-10-02","Docs")]:
        f.icon("file",RX2+2,ry+2,FAINT,14); f.t(RX2+24,ry,name,13,TEXT2,500); f.t(RX2+24,ry+18,where,11,TER); ry+=44
    statusbar(f,"Private pages are checked against your own pages only","Last pass Mon 09:00")
    return f

def s_home(x,y):
    f=F("0 · Knowledge home (new tab)",x,y)
    rail(f); explorer(f,active=None); tabs(f,["New tab"],"New tab",w=1440-MX)
    X=MX+(1440-MX-760)//2
    f.t(X,90,"Good morning, Liam",28,TEXT,700); f.t(X,130,"Thursday 13 November · 1,312 pages · 3 in your inbox",14,TER)
    bx=X; by=176
    for ic,lab,k,col in [("calendar","Today's note","⌘D",AMBER),("pen","New page","⌘N",ACC),("inbox","Quick capture","⌥⌘N",TEAL),("search","Go to page","⌘O",BLUE)]:
        f.rect(bx,by,182,92,fill=SURF,r=12,stroke=BORDER); f.icon(ic,bx+16,by+16,col,20)
        f.t(bx+16,by+52,lab,14,TEXT,600); f.kbd(bx+182-16-len(k)*7-10,by+16,k); bx+=192
    ry=300
    f.t(X,ry,"RECENT",10,TER,700); f.t(X+700,ry,"View all",12,ACCT,500); ry+=26
    for title,path,when,aud in [("The old harbour","Campaign / Locations","2 min ago","Private"),("2026-11-13","Daily","8 min ago","Private"),("Watchtower","Campaign / Locations","1 h ago","Private"),("Deploy runbook","Projects / Vesper","Yesterday","Shared"),("How to take smart notes","Literature","Yesterday","Private"),("Campaign locations","Campaign · saved query","Nov 11","Private")]:
        f.rect(X,ry,760,46,fill=None,r=8); f.line(X,ry+46,X+760,ry+46,"#181722")
        f.icon("table" if "query" in path else "file",X+8,ry+15,FAINT,15)
        f.t(X+34,ry+13,title,14,TEXT,500); f.t(X+300,ry+14,path,13,TER)
        f.icon("lock" if aud=="Private" else "share",X+590,ry+15,ACCT if aud=="Private" else BLUE,14); f.t(X+612,ry+14,aud,12,SEC)
        f.t(X+680,ry+14,when,12,TER,w=80,align="right"); ry+=46
    ry+=24
    f.rect(X,ry,374,112,fill=SURF,r=12,stroke=BORDER)
    f.icon("inbox",X+16,ry+16,TEAL,16); f.t(X+40,ry+14,"Inbox · 3 to file",14,TEXT,600)
    f.text(X+16,ry+44,340,[[("Ask Mara Vell why the bell rang twice",13,SEC)],[("Ferry strike during the festival",13,SEC)]],h=44)
    f.t(X+16,ry+88,"Open inbox →",12,ACCT,500)
    f.rect(X+386,ry,374,112,fill=SURF,r=12,stroke=BORDER)
    f.icon("upload",X+402,ry+16,ACCT,16); f.t(X+426,ry+14,"Bring your Obsidian vault",14,TEXT,600)
    f.text(X+402,ry+44,340,[[("Upload a zip. You see a dry run before anything is written, and can undo it.",13,SEC)]],h=40)
    f.t(X+402,ry+88,"Import vault →",12,ACCT,500)
    statusbar(f,"Your knowledge · private by default","Online over Tailscale")
    return f

def base_editor(f):
    f.k.extend(s_editor(0,0).k)
    f.rect(0,0,1440,900,fill="#05040ACC",name="Scrim")
ORDER=[s_home,s_editor,s_chat,s_review,s_graph,s_switcher,s_daily,s_query,s_import,s_import_states,s_agent,s_connect]
def build():
    out=[]
    for i,fn in enumerate(ORDER):
        x=(i%2)*1560; y=(i//2)*1020
        out.append(fn(x,y).node)
    return out
