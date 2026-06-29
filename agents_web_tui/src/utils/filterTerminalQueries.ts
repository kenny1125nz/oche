const reDECRQM = /\x1b\[\?(\d+)\$p/g
const reDSR = /\x1b\[6n/g
const reOSCFG = /\x1b\]10;\?\x07/g
const reOSCBG = /\x1b\]11;\?\x07/g
const reOSCClr = /\x1b\]4;\d+;\?\x07/g
const reWinPx = /\x1b\[14t/g
const reSynced = /\x1b\[\?2026[hl]/g
const reUnknownSet = /\x1b\[\?(?:2031|2027)[hl]/g
const reUnknownEnd = /\x1b\[\?u/g
const reCursorRestore = /\x1b\[u/g

export function filterTerminalQueries(text: string, respond?: (response: string) => void): string {
  text = text.replace(reDECRQM, (_match, mode) => {
    respond?.(`\x1b[?${mode};0$y`)
    return ''
  })

  text = text.replace(reDSR, '')

  text = text.replace(reOSCFG, '')
  text = text.replace(reOSCBG, '')
  text = text.replace(reOSCClr, '')
  text = text.replace(reWinPx, '')
  text = text.replace(reSynced, '')
  text = text.replace(reUnknownSet, '')
  text = text.replace(reUnknownEnd, '')
  text = text.replace(reCursorRestore, '')

  return text
}
