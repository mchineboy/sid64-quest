def on_enter(event):
    if event.room.name == "Silvervein Lift Landing":
        tell("The survey office holds the pump sequence. Watch for creatures in the side chambers. Attack only when ready.")

def on_command(event):
    if event.command != "crank" or event.room.name != "Silvervein Pump Chamber":
        return False
    if get_state("completed") == "yes":
        tell("Your survey case is already recovered. The pumps tick steadily.")
        return True
    order = ["intake", "wheel", "sluice"]
    choice = event.text.strip().lower()
    if choice not in order:
        tell("Use crank intake, crank wheel, or crank sluice.")
        return True
    step = int(get_state("step", "0"))
    if choice != order[step]:
        set_state("step", "0")
        tell("The mechanism disengages. Begin the sequence again; the office diagram explains the order.")
        return True
    step += 1
    if step == len(order):
        set_state("completed", "yes")
        set_state("step", "0")
        award_gold(40)
        tell("The pump clears its throat and the survey case opens. The recovery bounty is yours: 40 gold.")
    else:
        set_state("step", str(step))
        tell("The control locks into place. Set the next control.")
    return True
