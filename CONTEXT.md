# Homeoffice Tracker

A personal record of the days the owner works from home, so that the owner can count them per year for the tax report.

## Language

**Home office day**:
One calendar date on which the owner worked from home. A date is a home office day or it is not; there are no hours, half days, or duplicates.
_Avoid_: entry, record, booking

**Today**:
The current calendar date in the owner's local time zone.

**Past day**:
A calendar date before **Today** that the owner forgot to mark and adds later.

**Year count**:
The number of **Home office days** in one calendar year.
_Avoid_: total, sum

**Export**:
A CSV file that lists **Home office days**, which the owner can give to a tax advisor.

## Relationships

- The owner **marks** a date to make it a **Home office day**, and **unmarks** it to remove it.
- A **Year count** covers exactly one calendar year (1 January to 31 December).
- A **Home office day** is never after **Today**. Weekends and public holidays are allowed.
- An **Export** covers exactly one calendar year.

## Example dialogue

> **Owner:** "I forgot to mark last Tuesday."
> **Dev:** "Then you mark it as a **Past day**. The **Year count** for this year goes up by one."

## Flagged ambiguities

- None yet.
