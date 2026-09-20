def on_enter(event):
    tell("Beyond town: crypt, mine, and sea caves. Type trails for routes.")

def on_command(event):
    if event.command != "trails":
        return False
    tell("Bellkeeper Crypt: west, west from Town Square, then down. Seek a bronze-tongued riddle.")
    tell("Silvervein Mine: north, east, north, north, north, north, east, down. Restore the pumps.")
    tell("Tideglass Grotto: north, north, east, east, down. Relight the forgotten lens.")
    tell("Use up/down (u/d) on stairs. Every passage has a return exit. Read room descriptions for clues and puzzle commands.")
    tell("Rest at the Prancing Pony, Forester's Lodge, or Keeper's Cottage. Each dungeon offers a reward once per character.")
    tell("Hostile creatures occupy dungeon side chambers. Use attack <name> for one round, or leave by any exit. Weapons and armor help; defeat returns you to the inn.")
    return True
