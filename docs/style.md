# How this is written

The documents listed in `style_test.go` are written in what this repository
calls **plain technical English**. Every document in `docs/` is on that list.
A document joins it when somebody converts it, so a name in that list means
"this file has been read against the rules below", not "somebody intends to".

The README is not on the list. Its changelog is a record, and the rest of it
is the page the window renders in its About section.

The rules come from fynedesygn's `docs/style.md`, which takes them from
ASD-STE100, the Simplified Technical English specification written for
aerospace maintenance manuals. They are not that specification. STE controls
its vocabulary with a licensed dictionary, caps a procedural sentence at
twenty words, and allows one idea per sentence. This repository does none of
those three. **Nothing here should claim to be STE.** A reader who checks it
against the standard will find that it does not comply.

## The rules

**Use plain verbs, and use one verb for one meaning.** `reads`, `writes`,
`returns`, `draws`. Not `takes`, `speaks`, `grabs`, `punches`. A verb that
carries a metaphor is a verb the reader must translate first.

**Write in the active voice.** "The service renders the frame", not "the
frame is rendered by the service".

**Write in the simple present.** The behaviour is a fact about the program,
not a story about the day somebody found it.

**No idiom, no metaphor, no understatement.** "A table with a bend in it" and
"the honest answer" read well and cost a non-native reader a stop. Say what
happens.

**No asides in em-dashes, and no stacked subordinate clauses.** One sentence
may join two clauses with `and`, `so`, `because` or a semicolon. It should not
join four.

**Aim for twenty-five words and stop at thirty.** This is a ceiling, not a
target. A compound sentence that keeps cause and effect together is better
than two sentences that separate them.

**Keep the bold lead sentence.** It makes a long table skimmable, and plain
language does not mean flat formatting.

**Name the thing the same way every time.** A scene is a scene everywhere. It
is not a "preset" in one paragraph and a "profile" in the next.

**Define the words this project invented.** A *scene*, a *rule*, a *bucket*, a
*slot*, a *reading*, an *arrangement* and the *panel* are hotaru's own terms.
Use them, and define each one where a reader first meets it.

## What is not converted

**The specifications in `specs/`.** They record what was decided and why, at
the time it was decided. Rewriting them would change the record.

**The changelog in `README.md`.** It is the same kind of record.

**Doc comments in Go source.** They carry argument rather than instruction,
and that is the part these rules serve least well. A comment here says why a
constant is the number it is, or which bug a test was written for, and the
reasoning is the payload. The exemption is about fit, not about protecting a
house voice.

## What is checked

`style_test.go` holds the mechanical rules: the sentence ceiling, and a short
list of words that take a line's length and give nothing back. It also checks
that this page still disclaims compliance with STE. Everything else
on this page is judgement. A test that tried to enforce judgement would fail
more often than it helped, and somebody would turn it off within a month.
