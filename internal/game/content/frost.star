def on_enter(event):
    if event.room.name == "Rimeward Steps":
        tell("The verse wall holds the smith's firing order. Read it before working the levers at the forge.")

def on_command(event):
    if event.command != "stoke" or event.room.name != "Frozen Forge":
        return False
    if get_state("completed") == "yes":
        tell("Your fire is lit and the strongbox stands open.")
        return True
    order = ["tinder", "bellows", "flue"]
    choice = event.text.strip().lower()
    if choice not in order:
        tell("Use stoke tinder, stoke bellows, or stoke flue.")
        return True
    step = int(get_state("step", "0"))
    if choice != order[step]:
        set_state("step", "0")
        tell("The spark drowns in cold smoke. Begin again in the order carved on the verse wall.")
        return True
    step += 1
    if step == len(order):
        set_state("completed", "yes")
        set_state("step", "0")
        award_gold(60)
        tell("The hearth roars and meltwater runs from the strongbox hinges. It yields 60 gold.")
    else:
        set_state("step", str(step))
        tell("The lever moves and the cold loosens a little. Work the next lever.")
    return True
