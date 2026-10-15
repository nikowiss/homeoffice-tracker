# Homeoffice Tracker

A personal record of the days the owner works from home, so that the owner can count them per year for the tax report.

## Language

**Home office day**:
One calendar date on which the owner worked from home. A date is a home office day or it is not; there are no hours, half days, or duplicates.
_Avoid_: entry, record, booking

**Today**:
The current calendar date in the owner's local time zone.

**Past day**:
A calendar date before **Today**.

**Selection**:
The **Past days** that the owner chose to mark or unmark but has not confirmed yet. A **Selection** changes nothing until the owner confirms it.
_Avoid_: pending change, draft, batch

**Year count**:
The number of **Home office days** in one calendar year.
_Avoid_: total, sum

**Export**:
A CSV file that lists **Home office days**, which the owner can give to a tax advisor.

## Relationships

- The owner **marks** a date to make it a **Home office day**, and **unmarks** it to remove it.
- Marking or unmarking **Today** takes one action. Marking or unmarking a **Past day** needs a confirmation.
- A **Selection** can contain marked and unmarked **Past days** from more than one month. Confirming it unmarks each marked date and marks each unmarked date.
- Marking or unmarking **Today** does not change the **Selection**.
- A **Year count** covers exactly one calendar year (1 January to 31 December).
- A **Home office day** is never after **Today**. Weekends and public holidays are allowed.
- An **Export** covers exactly one calendar year.

## Example dialogue

> **Owner:** "I forgot to mark last Tuesday."
> **Dev:** "Then you mark it as a **Past day**. The **Year count** for this year goes up by one."
> **Owner:** "I also forgot Monday, and I marked Wednesday by mistake."
> **Dev:** "Put all three into one **Selection** and confirm it. Monday and Tuesday become **Home office days**, and Wednesday is unmarked."

## Flagged ambiguities

- None yet.
