def on_enter(event):
    if event.room.name == "Tithe Stair":
        tell("The ledger alcove records the tithe order. Read it before working the plates in the strongroom.")

def on_command(event):
    if event.command != "press" or event.room.name != "Tithe Strongroom":
        return False
    if get_state("completed") == "yes":
        tell("Your share is already counted. The tithe box rests open and empty.")
        return True
    order = ["grain", "coin", "seal"]
    choice = event.text.strip().lower()
    if choice not in order:
        tell("Use press grain, press coin, or press seal.")
        return True
    step = int(get_state("step", "0"))
    if choice != order[step]:
        set_state("step", "0")
        tell("The measures rattle and empty themselves. Begin again in the order kept by the ledger.")
        return True
    step += 1
    if step == len(order):
        set_state("completed", "yes")
        set_state("step", "0")
        award_gold(45)
        tell("The seal sinks home and the tithe box opens. Your honest share is 45 gold.")
    else:
        set_state("step", str(step))
        tell("The plate settles flush with the table. Press the next plate.")
    return True
