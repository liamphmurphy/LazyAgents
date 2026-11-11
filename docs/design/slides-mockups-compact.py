import re, sys
for p in sys.argv[1:]:
    s = open(p).read()
    F = "font-family=\"Inter, 'Helvetica Neue', Arial, sans-serif\""
    s = s.replace(" " + F, "")
    s = s.replace('<svg xmlns="http://www.w3.org/2000/svg"', '<svg xmlns="http://www.w3.org/2000/svg" ' + F, 1)
    for a in [' font-weight="400"', ' text-anchor="start"', ' rx="0"', ' stroke-width="1"', ' x="0" y="0"']:
        s = s.replace(a, "")
    s = re.sub(r"\s+/>", "/>", s)
    s = re.sub(r" >", ">", s)
    open(p, "w").write(s)
