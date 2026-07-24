#!/bin/bash
set -euo pipefail
workspace_dir="${WORKSPACE_DIR:-/workspace}"
if [ -d "$workspace_dir" ]; then cd "$workspace_dir"; fi
tmp_patch="$(mktemp)"
cleanup() { rm -f "$tmp_patch"; }
trap cleanup EXIT
cat > "$tmp_patch" <<'CODEBUDDY_PATCH'
diff --git a/httpx_like/request_model.py b/httpx_like/request_model.py
index 385bdaf..52bc612 100644
--- a/httpx_like/request_model.py
+++ b/httpx_like/request_model.py
@@ -1,4 +1,25 @@
 class RequestBuilder:
     def __init__(self, base_headers=None, cookies=None, params=None): self.base_headers=base_headers or {}; self.cookies=cookies or {}; self.params=params or {}
+    def _merge_headers(self, headers):
+        merged={k.lower():v for k,v in self.base_headers.items()}
+        for k,v in (headers or {}).items(): merged[k.lower()]=v
+        return merged
+    def _merge_mapping(self, base, override): merged=dict(base); merged.update(override or {}); return merged
+    def _merge_params(self, params):
+        out=[]
+        for source in (self.params, params or {}):
+            items=source.items() if hasattr(source,"items") else source
+            for k,v in items:
+                if v is None: continue
+                if isinstance(v,(list,tuple)): out.extend((k,x) for x in v if x is not None)
+                else: out.append((k,v))
+        return out
+    def _body(self, json=None, data=None, content=None):
+        active=[(n,v) for n,v in (("json",json),("data",data),("content",content)) if v is not None]
+        if len(active)>1: raise ValueError("json, data, and content are mutually exclusive")
+        return active[0] if active else (None,None)
     def build(self, method, url, headers=None, cookies=None, params=None, json=None, data=None, content=None):
-        return {"method":method,"url":url,"headers":dict(headers or {}),"cookies":dict(cookies or {}),"params":dict(params or {}),"body":json or data or content}
+        body_type, body = self._body(json=json, data=data, content=content)
+        hdr=self._merge_headers(headers)
+        if body_type=="json" and "content-type" not in hdr: hdr["content-type"]="application/json"
+        return {"method":method.upper(),"url":url,"headers":hdr,"cookies":self._merge_mapping(self.cookies,cookies),"params":self._merge_params(params),"body_type":body_type,"body":body}
CODEBUDDY_PATCH
git apply "$tmp_patch"
