def on_enter(event):
    tell("Beyond town: crypt, mine, sea caves, farm country, a wrecking coast, and the high country. Type trails for routes.")

def on_command(event):
    if event.command != "trails":
        return False
    tell("Bellkeeper Crypt: west, west from Town Square, then down. Seek a bronze-tongued riddle.")
    tell("Silvervein Mine: north, east, north, north, north, north, east, down. Restore the pumps.")
    tell("Tideglass Grotto: north, north, east, east, down. Relight the forgotten lens.")
    tell("Eastvale and the Tithe Vault: reach Willow Bridge (north, east, north), then east, east, east, north, east, down. Count the tithe in order.")
    tell("Stormbreak and the Tideworn Hulk: reach Saltwind Path (south, south, south), then south, south, south, east, south, down. Rig the capstan.")
    tell("The Highland and Frostfall Deep: reach the Broken Watchtower, then up, north, north, east, north, down. Wake the frozen forge.")
    tell("Shortcuts: the orchard runs east to Eastvale, the ridge north to the high country, the keeper's cottage south to the dunes, and the mine entrance east across the old toll span.")
    tell("The Hall of Returning lies east of the crypt lantern stair. Wardens keep it for the fallen; no creature enters it.")
    tell("Use up/down (u/d) on stairs. Every passage has a return exit. Read room descriptions for clues and puzzle commands.")
    tell("Rest at the Prancing Pony, Forester's Lodge, Keeper's Cottage, Harrowgate Grange, the Wrecker's Hut, or the Shepherd's Bothy. Each dungeon offers a reward once per character.")
    tell("Hostile creatures occupy dungeon side chambers. Use attack <name>, then loot <name> after victory. Rare armor can be found on corpses.")
    tell("Death carries you to the Hall of Returning. Pay 100 gold with resurrect pay, or wait ten minutes and use resurrect.")
    return True
