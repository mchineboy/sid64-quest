def on_enter(event):
    if event.room.name == "Wrackstair":
        tell("The galley table carries the purser's rigging order. Read it before working the capstan pins.")

def on_command(event):
    if event.command != "rig" or event.room.name != "Capstan Deck":
        return False
    if get_state("completed") == "yes":
        tell("Your rigging holds. The purser's chest has already paid you out.")
        return True
    order = ["anchor", "spar", "sail"]
    choice = event.text.strip().lower()
    if choice not in order:
        tell("Use rig anchor, rig spar, or rig sail.")
        return True
    step = int(get_state("step", "0"))
    if choice != order[step]:
        set_state("step", "0")
        tell("The line runs out and the capstan spins free. Begin again from the anchor.")
        return True
    step += 1
    if step == len(order):
        set_state("completed", "yes")
        set_state("step", "0")
        award_gold(55)
        tell("Canvas cracks open above the flats and the chest springs its lashings: 55 gold is yours.")
    else:
        set_state("step", str(step))
        tell("The pin bites and the line goes taut. Work the next pin.")
    return True
