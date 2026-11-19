# Reads the newest letter for an address from the Mailpit test mail server.
# usage: python mp.py <to-address> [field]   field = subject | subjects | replyto | files | text | link | count
# MAILPIT_API overrides the address (the Kafka e2e uses a second Mailpit on another port).
import json, os, re, sys, urllib.request

API = os.environ.get("MAILPIT_API", "http://127.0.0.1:8025/api/v1")
to, field = sys.argv[1], (sys.argv[2] if len(sys.argv) > 2 else "subject")

def get(path):
    with urllib.request.urlopen(API + path) as r:
        return json.load(r)

msgs = [m for m in get("/messages?limit=200")["messages"]
        if any(a["Address"] == to for a in m["To"])]
if not msgs:
    print("NONE")
    sys.exit()
m = get("/message/" + msgs[0]["ID"])  # newest first
if field == "subjects":  # every letter to this address, oldest first
    print(" | ".join(x["Subject"] for x in reversed(msgs)))
elif field == "subject":
    print(m["Subject"])
elif field == "replyto":
    print(",".join(a["Address"] for a in m["ReplyTo"]))
elif field == "files":
    print(",".join(a["FileName"] for a in m["Attachments"]))
elif field == "count":
    print(len(msgs))
elif field == "link":  # first http link in the letter (verification / password reset)
    found = re.search(r'https?://[^\s"<>]+', m["HTML"] or m["Text"])
    print(found.group(0).replace("&amp;", "&") if found else "NONE")
else:
    print(m["Text"])
