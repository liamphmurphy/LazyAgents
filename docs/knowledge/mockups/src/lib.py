import json
# Vesper Dusk tokens (web/styles/tokens.css)
BG="#0C0B12"; SIDE="#100F18"; PANEL="#0E0D15"; SURF="#15141F"; RAISED="#1B1A27"; SUNK="#110F19"
HOVER="#1A1925"; SEL="#201E2C"; BORDER="#24222F"; SUBTLE="#1E1C29"; STRONG="#2E2B3B"
TEXT="#EEEDF3"; TEXT2="#CBC9D6"; SEC="#A4A1B3"; TER="#8C899C"; FAINT="#5E5B6E"
ACC="#9B8CFF"; ACCS="#9B8CFF26"; ACCT="#B9AEFF"; STAR="#F2A97E"; DEEP="#2B2366"
GREEN="#4CB782"; GREENS="#4CB78233"; RED="#EF6461"; REDS="#EF646133"; AMBER="#F0BF4C"; AMBERS="#F0BF4C24"
BLUE="#4EA7FC"; BLUES="#4EA7FC24"; ORANGE="#F59E5B"; ORANGES="#F59E5B24"; TEAL="#3FC1B0"; PINK="#F48A98"

class F:
    def __init__(s, name, x, y, w=1440, h=900):
        s.node={"type":"frame","name":name,"x":x,"y":y,"w":w,"h":h,"fill":BG,"clip":True,"radius":12,"children":[]}
        s.k=s.node["children"]
    def add(s,n): s.k.append(n); return n
    def rect(s,x,y,w,h,fill=None,stroke=None,r=0,sw=1,name=None,dash=None):
        n={"type":"shape","shape":"roundRect" if r else "rect","x":x,"y":y,"w":w,"h":h}
        if r: n["radius"]=r
        if fill: n["fill"]=fill
        if stroke: n["stroke"]=stroke; n["stroke_width"]=sw
        if dash: n["dash"]=dash
        if name: n["name"]=name
        return s.add(n)
    def dot(s,x,y,d,fill,stroke=None,sw=1):
        n={"type":"shape","shape":"ellipse","x":x,"y":y,"w":d,"h":d,"fill":fill}
        if stroke: n["stroke"]=stroke; n["stroke_width"]=sw
        return s.add(n)
    def line(s,x1,y1,x2,y2,color=BORDER,sw=1,dash=None):
        x,y=min(x1,x2),min(y1,y2)
        n={"type":"path","x":x,"y":y,"w":max(1,abs(x2-x1)),"h":max(1,abs(y2-y1)),"d":f"M {x1-x} {y1-y} L {x2-x} {y2-y}","stroke":color,"stroke_width":sw,"cap":"round"}
        if dash: n["dash"]=dash
        return s.add(n)
    def text(s,x,y,w,runs,h=None,align=None,valign=None,name=None,ls=None):
        """runs: str | list of paragraphs; paragraph = list of (text,size,color,weight,extra)"""
        paras=runs if isinstance(runs,list) else [[(runs,)]]
        ps=[]; size0=13
        for p in paras:
            rs=[]
            for r in p:
                t=r[0]; sz=r[1] if len(r)>1 else 13; c=r[2] if len(r)>2 else TEXT2; wt=r[3] if len(r)>3 else 400
                run={"text":t,"size":sz,"color":c}
                if wt!=400: run["weight"]=wt
                if len(r)>4: run.update(r[4])
                rs.append(run); size0=sz
            para={"runs":rs}
            if align: para["align"]=align
            if ls: para["lineSpacing"]=ls
            ps.append(para)
        n={"type":"text","x":x,"y":y,"w":w,"h":h or int(size0*1.5*len(ps))+4,"paragraphs":ps}
        if valign: n["valign"]=valign
        if name: n["name"]=name
        return s.add(n)
    def t(s,x,y,txt,size=13,color=TEXT2,weight=400,w=None,align=None,**kw):
        return s.text(x,y,w or max(24,int(len(txt)*size*0.68)+10),[[(txt,size,color,weight,kw)]] if kw else [[(txt,size,color,weight)]],align=align)
    def icon(s,name,x,y,color=SEC,size=16,sw=1.5):
        d=ICONS[name]; k=size/16
        import re
        d=re.sub(r"(-?\d+(?:\.\d+)?) (-?\d+(?:\.\d+)?)",lambda m:f"{round(float(m.group(1))*k,2)} {round(float(m.group(2))*k,2)}",d)
        return s.add({"type":"path","name":"icon "+name,"x":x,"y":y,"w":size,"h":size,"d":d,"stroke":color,"stroke_width":sw,"cap":"round","join":"round"})
    def pill(s,x,y,txt,fg=ACCT,bg=ACCS,size=11,h=20,pad=8,r=10,weight=500,stroke=None):
        w=int(len(txt)*size*0.6)+2*pad
        s.rect(x,y,w,h,fill=bg,r=r,stroke=stroke)
        s.t(x+pad,y+(h-size*1.3)/2,txt,size,fg,weight,w=w)
        return w
    def button(s,x,y,txt,primary=False,w=None,h=30,icon=None,size=13):
        w=w or int(len(txt)*size*0.6)+24+(20 if icon else 0)
        if primary: s.rect(x,y,w,h,fill=ACC,r=7)
        else: s.rect(x,y,w,h,fill=SURF,stroke=STRONG,r=7)
        fg="#120F24" if primary else TEXT2
        tx=x+12
        if icon: s.icon(icon,x+10,y+(h-16)/2,fg); tx=x+32
        s.t(tx,y+(h-size*1.35)/2,txt,size,fg,600 if primary else 500,w=w-(tx-x))
        return w
    def kbd(s,x,y,txt):
        w=int(len(txt)*7)+10
        s.rect(x,y,w,18,fill=RAISED,stroke=STRONG,r=4); s.t(x+5,y+2,txt,11,SEC,500,w=w)
        return w

# 16x16 stroke icons, absolute M/L/C/Z only
ICONS={
 "search":"M 7 2.5 C 9.5 2.5 11.5 4.5 11.5 7 C 11.5 9.5 9.5 11.5 7 11.5 C 4.5 11.5 2.5 9.5 2.5 7 C 2.5 4.5 4.5 2.5 7 2.5 Z M 10.3 10.3 L 13.5 13.5",
 "file":"M 4 1.5 L 9.5 1.5 L 12.5 4.5 L 12.5 14.5 L 4 14.5 Z M 9.5 1.5 L 9.5 4.5 L 12.5 4.5",
 "folder":"M 1.5 4 L 1.5 13 L 14.5 13 L 14.5 5.5 L 7.5 5.5 L 6 3.5 L 1.5 3.5 Z",
 "chev-r":"M 6 4 L 10 8 L 6 12",
 "chev-d":"M 4 6 L 8 10 L 12 6",
 "plus":"M 8 3 L 8 13 M 3 8 L 13 8",
 "x":"M 4 4 L 12 12 M 12 4 L 4 12",
 "graph":"M 4 4 L 12 6 L 7 12 Z M 3 2.5 L 5 2.5 L 5 5.5 L 3 5.5 Z M 11 4.5 L 13 4.5 L 13 7.5 L 11 7.5 Z M 6 10.5 L 8 10.5 L 8 13.5 L 6 13.5 Z",
 "calendar":"M 2.5 3.5 L 13.5 3.5 L 13.5 14 L 2.5 14 Z M 2.5 6.5 L 13.5 6.5 M 5.5 2 L 5.5 5 M 10.5 2 L 10.5 5",
 "inbox":"M 2 9 L 5 9 L 6 11 L 10 11 L 11 9 L 14 9 M 2 9 L 4 3 L 12 3 L 14 9 L 14 13.5 L 2 13.5 Z",
 "link":"M 6.5 9.5 L 9.5 6.5 M 7 4.5 L 8.5 3 C 9.8 1.7 12 1.7 13.2 3 C 14.4 4.3 14.4 6.4 13.2 7.6 L 11.5 9.2 M 9 11.5 L 7.5 13 C 6.2 14.3 4 14.3 2.8 13 C 1.6 11.7 1.6 9.6 2.8 8.4 L 4.5 6.8",
 "list":"M 5.5 4 L 14 4 M 5.5 8 L 14 8 M 5.5 12 L 14 12 M 2 4 L 2.5 4 M 2 8 L 2.5 8 M 2 12 L 2.5 12",
 "chat":"M 2.5 3 L 13.5 3 L 13.5 11 L 7 11 L 4 13.5 L 4 11 L 2.5 11 Z",
 "lock":"M 3.5 7.5 L 12.5 7.5 L 12.5 14 L 3.5 14 Z M 5.5 7.5 L 5.5 5 C 5.5 3.5 6.5 2.5 8 2.5 C 9.5 2.5 10.5 3.5 10.5 5 L 10.5 7.5",
 "share":"M 8 2 L 8 10 M 5 5 L 8 2 L 11 5 M 3 8.5 L 3 14 L 13 14 L 13 8.5",
 "bot":"M 3 5.5 L 13 5.5 L 13 13 L 3 13 Z M 8 5.5 L 8 3 M 6 9 L 6 9.5 M 10 9 L 10 9.5",
 "upload":"M 8 10.5 L 8 2.5 M 5 5.5 L 8 2.5 L 11 5.5 M 2.5 10 L 2.5 13.5 L 13.5 13.5 L 13.5 10",
 "check":"M 3 8.5 L 6.5 12 L 13 4.5",
 "alert":"M 8 2 L 14.5 13.5 L 1.5 13.5 Z M 8 6.5 L 8 9.5 M 8 11.5 L 8 11.6",
 "info":"M 8 1.8 C 11.4 1.8 14.2 4.6 14.2 8 C 14.2 11.4 11.4 14.2 8 14.2 C 4.6 14.2 1.8 11.4 1.8 8 C 1.8 4.6 4.6 1.8 8 1.8 Z M 8 7.5 L 8 11 M 8 5 L 8 5.1",
 "star":"M 8 1.5 L 9.9 5.6 L 14.3 6 L 11 9 L 12 13.4 L 8 11.1 L 4 13.4 L 5 9 L 1.7 6 L 6.1 5.6 Z",
 "table":"M 2 3 L 14 3 L 14 13 L 2 13 Z M 2 6.5 L 14 6.5 M 6.5 3 L 6.5 13",
 "sparkle":"M 8 1.5 L 9.2 6.8 L 14.5 8 L 9.2 9.2 L 8 14.5 L 6.8 9.2 L 1.5 8 L 6.8 6.8 Z",
 "clock":"M 8 1.8 C 11.4 1.8 14.2 4.6 14.2 8 C 14.2 11.4 11.4 14.2 8 14.2 C 4.6 14.2 1.8 11.4 1.8 8 C 1.8 4.6 4.6 1.8 8 1.8 Z M 8 4.5 L 8 8 L 10.5 9.5",
 "tag":"M 2 2 L 8 2 L 14 8 L 8 14 L 2 8 Z M 5 5 L 5.1 5",
 "more":"M 3 8 L 3.1 8 M 8 8 L 8.1 8 M 13 8 L 13.1 8",
 "pen":"M 10.5 2.5 L 13.5 5.5 L 5.5 13.5 L 2.5 13.5 L 2.5 10.5 Z",
 "pause":"M 5.5 3 L 5.5 13 M 10.5 3 L 10.5 13",
 "home":"M 2 7.5 L 8 2 L 14 7.5 M 3.5 6.5 L 3.5 14 L 12.5 14 L 12.5 6.5",
 "board":"M 2 2.5 L 14 2.5 L 14 13.5 L 2 13.5 Z M 6 2.5 L 6 13.5 M 10 2.5 L 10 13.5",
 "zap":"M 9 1.5 L 3 9 L 8 9 L 7 14.5 L 13 7 L 8 7 Z",
 "book":"M 2 3 L 6.5 3 C 7.5 3 8 3.5 8 4.5 L 8 14 C 8 13 7.5 12.5 6.5 12.5 L 2 12.5 Z M 14 3 L 9.5 3 C 8.5 3 8 3.5 8 4.5 M 14 3 L 14 12.5 L 9.5 12.5 C 8.5 12.5 8 13 8 14",
 "undo":"M 5 3.5 L 2 6.5 L 5 9.5 M 2 6.5 L 10 6.5 C 12.5 6.5 14 8 14 10 C 14 12 12.5 13.5 10 13.5 L 6 13.5",
 "filter":"M 2 3 L 14 3 L 9.5 8.5 L 9.5 13.5 L 6.5 12 L 6.5 8.5 Z",
 "settings":"M 8 5.5 C 9.4 5.5 10.5 6.6 10.5 8 C 10.5 9.4 9.4 10.5 8 10.5 C 6.6 10.5 5.5 9.4 5.5 8 C 5.5 6.6 6.6 5.5 8 5.5 Z M 8 1.5 L 8 3.5 M 8 12.5 L 8 14.5 M 1.5 8 L 3.5 8 M 12.5 8 L 14.5 8 M 3.4 3.4 L 4.8 4.8 M 11.2 11.2 L 12.6 12.6 M 3.4 12.6 L 4.8 11.2 M 11.2 4.8 L 12.6 3.4",
 "sidebar":"M 2 2.5 L 14 2.5 L 14 13.5 L 2 13.5 Z M 10 2.5 L 10 13.5",
}
