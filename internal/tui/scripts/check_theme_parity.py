import os
import re

GO_THEMES_DIR = r"c:\Users\CLHat\Documents\GitHub\GhostWriters\DockSTARTer2\internal\assets\themes"
BASH_THEMES_DIR = r"c:\Users\CLHat\Documents\GitHub\GhostWriters\DockSTARTer\assets\themes"

def parse_ds2theme(path):
    colors = {}
    if not os.path.exists(path):
        return colors
    with open(path, 'r') as f:
        for line in f:
            if '=' in line:
                key, val = line.split('=', 1)
                colors[key.strip()] = val.strip().strip("'\"")
    return colors

def parse_dialogrc(path):
    colors = {}
    if not os.path.exists(path):
        return colors
    with open(path, 'r') as f:
        for line in f:
            if '=' in line and '(' in line:
                key, val = line.split('=', 1)
                # (FG,BG,HL)
                match = re.search(r'\(([^)]+)\)', val)
                if match:
                    colors[key.strip()] = match.group(1).split(',')
    return colors

def parse_theme_ini(path):
    vals = {}
    if not os.path.exists(path):
        return vals
    with open(path, 'r') as f:
        for line in f:
            if '=' in line and not line.startswith('#'):
                key, val = line.split('=', 1)
                vals[key.strip()] = val.strip().strip("'\"")
    return vals

def check_parity(name):
    print(f"--- Checking {name} ---")
    go_path = os.path.join(GO_THEMES_DIR, f"{name}.ds2theme")
    bash_ini_path = os.path.join(BASH_THEMES_DIR, name, "theme.ini")
    bash_rc_path = os.path.join(BASH_THEMES_DIR, name, ".dialogrc")

    go = parse_ds2theme(go_path)
    ini = parse_theme_ini(bash_ini_path)
    rc = parse_dialogrc(bash_rc_path)

    # Check headers
    # In bash: Title="${_NC_}${_U_}"
    # In Go: Title="{{|white:black:U|}}"
    # We need to map bash variables to colors based on screen background
    screen_bg = "black"
    if 'screen_color' in rc:
        screen_bg = rc['screen_color'][1].strip().lower()

    def get_bash_color(val):
        # Very simple mapper for this context
        res = "NC"
        if "${_W_}" in val: res = "white"
        elif "${_G_}" in val: res = "green"
        elif "${_C_}" in val: res = "cyan"
        elif "${_M_}" in val: res = "magenta"
        elif "${_Y_}" in val: res = "yellow"
        elif "${_R_}" in val: res = "red"
        elif "${_B_}" in val: res = "blue"
        elif "${_K_}" in val: res = "black"
        
        is_rev = "${_RV_}" in val
        return res, is_rev

    for k in ["Title", "Subtitle", "TitleSuccess", "TitleError", "TitleWarning"]:
        if k in ini and k in go:
            b_col, b_rev = get_bash_color(ini[k])
            # If reverse, text is screen background color? No, NC reverse is FG=BG BG=FG?
            # NC is FG=NC BG=NC. 
            # In GreenScreen: Screen=(BLACK,GREEN,ON) -> FG=Black, BG=Green.
            # NC Reverse -> FG=Green, BG=Black.
            pass

    # Simplified: Just print them for me to look at
    print(f"Go Title: {go.get('Title')}")
    print(f"Bash Title: {ini.get('Title')}")
    print(f"Bash dialogrc Title: {rc.get('title_color')}")
    print(f"Bash dialogrc Screen: {rc.get('screen_color')}")
    print()

themes = [d for d in os.listdir(BASH_THEMES_DIR) if os.path.isdir(os.path.join(BASH_THEMES_DIR, d))]
for t in themes:
    check_parity(t)
