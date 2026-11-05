"""Preserve user-added preview sounds when returning to the normal app profile."""
from pathlib import Path
import copy
import hashlib
import json
import os
import shutil
import tempfile
import uuid

workspace = Path(__file__).resolve().parent.parent
source = workspace / ".qa-profile"
destination = Path(os.environ["APPDATA"]) / "QuietLab"

def read(path, fallback):
    return json.loads(path.read_text(encoding="utf-8-sig")) if path.exists() else fallback

def write(path, data):
    with tempfile.NamedTemporaryFile(mode="w", encoding="utf-8", dir=path.parent, delete=False) as f:
        json.dump(data, f, ensure_ascii=False, indent=2)
        temp = f.name
    os.replace(temp, path)

def audio_path(root, name):
    if not name or "/" in name or "\\" in name or Path(name).name != name:
        raise ValueError("Invalid stored audio filename")
    return root / "sounds" / name

destination.mkdir(parents=True, exist_ok=True)
backup = workspace / ".profile-backup" / uuid.uuid4().hex
backup.mkdir(parents=True)
for name in ("soundboards.json", "preferences.json"):
    path = destination / name
    if path.exists():
        shutil.copy2(path, backup / name)
library = read(destination / "soundboards.json", {"Boards": []})
preview = read(source / "soundboards.json", {"Boards": []})
prefs = read(destination / "preferences.json", {})
preview_prefs = read(source / "preferences.json", {})
known = {}
for board in library["Boards"]:
    for sound in board["Sounds"]:
        path = audio_path(destination, sound["File"])
        if path.exists():
            known[(sound["Name"], hashlib.sha256(path.read_bytes()).hexdigest())] = board["ID"]
if not prefs.get("Options", {}).get("Filters") and preview_prefs.get("Options", {}).get("Filters"):
    prefs.setdefault("Options", {})["Filters"] = copy.deepcopy(preview_prefs["Options"]["Filters"])
    prefs["Options"]["BypassFilters"] = preview_prefs["Options"].get("BypassFilters", False)
    prefs["FilterGroups"] = copy.deepcopy(preview_prefs.get("FilterGroups", []))
    prefs["AllFilterHotkeys"] = copy.deepcopy(preview_prefs.get("AllFilterHotkeys", []))
    prefs["AllFilterMIDIs"] = copy.deepcopy(preview_prefs.get("AllFilterMIDIs", []))
existing_keys = set()
existing_midi = set()
def bindings(item):
    keys = item.get("Hotkeys")
    midi = item.get("MIDIs")
    return (keys if keys is not None else [item["Hotkey"]] if item.get("Hotkey") else [],
            midi if midi is not None else [item["MIDI"]] if item.get("MIDI") else [])
def remember(item):
    keys, midi = bindings(item)
    existing_keys.update((k["Modifiers"], k["Key"]) for k in keys)
    existing_midi.update((m["Kind"], m["Channel"], m["Number"]) for m in midi)
for board in library["Boards"]:
    for sound in board["Sounds"]:
        remember(sound)
for filt in prefs.get("Options", {}).get("Filters", []):
    remember(filt)
for group in prefs.get("FilterGroups") or []:
    remember(group)
remember({"Hotkeys": prefs.get("AllFilterHotkeys", []), "MIDIs": prefs.get("AllFilterMIDIs", [])})
count = 0
for board in preview["Boards"]:
    new_board = {"ID": uuid.uuid4().hex, "Name": "Preview sounds" if board["Name"] == "QA soundboard" else board["Name"], "Sounds": []}
    for sound in board["Sounds"]:
        # The original generated QA fixture is not user content.
        if sound["Name"] == "Quiet test clip" and sound["ID"] == "qa-clip":
            continue
        raw = audio_path(source, sound["File"]).read_bytes()
        signature = (sound["Name"], hashlib.sha256(raw).hexdigest())
        if signature in known:
            continue
        new = copy.deepcopy(sound)
        new["ID"] = uuid.uuid4().hex
        new["File"] = new["ID"] + Path(sound["File"]).suffix
        keys, midi = bindings(sound)
        new["Hotkeys"] = [k for k in keys if (k["Modifiers"], k["Key"]) not in existing_keys]
        new["MIDIs"] = [m for m in midi if (m["Kind"], m["Channel"], m["Number"]) not in existing_midi]
        new["Hotkey"] = new["MIDI"] = None
        remember(new)
        (destination / "sounds").mkdir(exist_ok=True)
        audio_path(destination, new["File"]).write_bytes(raw)
        new_board["Sounds"].append(new)
        known[signature] = new_board["ID"]
        count += 1
    if new_board["Sounds"]:
        library["Boards"].append(new_board)
        prefs["BoardID"] = new_board["ID"]
write(destination / "soundboards.json", library)
write(destination / "preferences.json", prefs)
print(f"Preserved {count} preview sound(s). Original profile files backed up locally.")
