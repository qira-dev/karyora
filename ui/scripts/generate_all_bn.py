import json
import os
import re
import time
import urllib.parse
import urllib.request

EN_DIR = "/home/km-abdullah/karyora_Platform/karyora/ui/src/i18n/locales/en"
BN_DIR = "/home/km-abdullah/karyora_Platform/karyora/ui/src/i18n/locales/bn"

os.makedirs(BN_DIR, exist_ok=True)

def has_bengali(text):
    return bool(re.search(r"[\u0980-\u09FF]", text))

def translate_batch(texts):
    if not texts:
        return []
    
    # Strip any internal newlines to keep line count consistent
    cleaned = [t.replace("\n", " ").strip() for t in texts]
    joined = "\n".join(cleaned)
    
    # Mask {{var}} and {var}
    vars_found = re.findall(r"\{\{.*?\}\}|\{.*?\}", joined)
    masked = joined
    for i, v in enumerate(vars_found):
        masked = masked.replace(v, f"XYZVAR{i}XYZ", 1)
        
    url = "https://translate.googleapis.com/translate_a/single?client=gtx&sl=en&tl=bn&dt=t&q=" + urllib.parse.quote(masked)
    req = urllib.request.Request(url, headers={"User-Agent": "Mozilla/5.0"})
    
    res = ""
    for attempt in range(3):
        try:
            with urllib.request.urlopen(req, timeout=10) as resp:
                data = json.loads(resp.read().decode("utf-8"))
                res = "".join(part[0] for part in data[0] if part[0])
            break
        except Exception as e:
            time.sleep(1.0 * (attempt + 1))
            if attempt == 2:
                print(f"Batch translation error: {e}")
                return texts
                
    # Unmask variables
    for i, v in enumerate(vars_found):
        res = re.sub(rf"XYZ\s*VAR{i}\s*XYZ", v, res)
        
    parts = [p.strip() for p in res.split("\n")]
    if len(parts) == len(texts):
        return parts
        
    print(f"Warning: line count mismatch (expected {len(texts)}, got {len(parts)}). Translating individually...")
    singles = []
    for t in cleaned:
        singles.append(translate_single(t))
    return singles

def translate_single(text):
    vars_found = re.findall(r"\{\{.*?\}\}|\{.*?\}", text)
    masked = text
    for i, v in enumerate(vars_found):
        masked = masked.replace(v, f"XYZVAR{i}XYZ", 1)
        
    url = "https://translate.googleapis.com/translate_a/single?client=gtx&sl=en&tl=bn&dt=t&q=" + urllib.parse.quote(masked)
    req = urllib.request.Request(url, headers={"User-Agent": "Mozilla/5.0"})
    try:
        with urllib.request.urlopen(req, timeout=8) as resp:
            data = json.loads(resp.read().decode("utf-8"))
            res = "".join(part[0] for part in data[0] if part[0])
        for i, v in enumerate(vars_found):
            res = re.sub(rf"XYZ\s*VAR{i}\s*XYZ", v, res)
        return res.strip()
    except Exception:
        return text

def collect_keys(obj, prefix=""):
    items = []
    for k, v in obj.items():
        p = f"{prefix}.{k}" if prefix else k
        if isinstance(v, dict):
            items.extend(collect_keys(v, p))
        else:
            items.append((p, v))
    return items

def set_key(obj, path, value):
    parts = path.split(".")
    curr = obj
    for p in parts[:-1]:
        if p not in curr or not isinstance(curr[p], dict):
            curr[p] = {}
        curr = curr[p]
    curr[parts[-1]] = value

def get_key(obj, path):
    parts = path.split(".")
    curr = obj
    for p in parts:
        if not isinstance(curr, dict) or p not in curr:
            return None
        curr = curr[p]
    return curr

def process_file(filename):
    en_path = os.path.join(EN_DIR, filename)
    bn_path = os.path.join(BN_DIR, filename)
    
    with open(en_path, "r", encoding="utf-8") as f:
        en_data = json.load(f)
        
    bn_data = {}
    if os.path.exists(bn_path):
        try:
            with open(bn_path, "r", encoding="utf-8") as f:
                bn_data = json.load(f)
        except Exception:
            bn_data = {}
            
    en_entries = collect_keys(en_data)
    to_translate = []
    
    for path, en_val in en_entries:
        existing_val = get_key(bn_data, path)
        if existing_val and isinstance(existing_val, str) and has_bengali(existing_val):
            continue
        to_translate.append((path, en_val))
        
    print(f"{filename}: {len(to_translate)} strings to translate out of {len(en_entries)}")
    
    chunk_size = 15
    for i in range(0, len(to_translate), chunk_size):
        chunk = to_translate[i:i+chunk_size]
        paths = [p for p, _ in chunk]
        texts = [t for _, t in chunk]
        
        translated = translate_batch(texts)
        for p, trans in zip(paths, translated):
            set_key(bn_data, p, trans)
        time.sleep(0.2)
        
    with open(bn_path, "w", encoding="utf-8") as f:
        json.dump(bn_data, f, indent=2, ensure_ascii=False)
    print(f"Saved {bn_path}")

def main():
    files = sorted([f for f in os.listdir(EN_DIR) if f.endswith(".json")])
    for f in files:
        process_file(f)

if __name__ == "__main__":
    main()
