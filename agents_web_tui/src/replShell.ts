export interface TermWriter {
  write(data: string): void
  clear(): void
  cols: number
  rows: number
}

export class ReplShell {
  busy = false
  currentLine = ''
  cmdQueue: string[] = []
  prompt = ''
  pendingRepl: { sessionId: string } | null = null
  interactive = false
  history: string[] = []
  historyIndex = -1
  cursorPos = 0

  constructor(
    private term: TermWriter,
    private sendRaw: (msg: string) => void,
  ) {}

  writePrompt() {
    this.term.write(this.prompt)
  }

  rewritePrompt(prompt: string) {
    const old = this.prompt
    this.prompt = prompt
    if (!this.busy && this.currentLine === '') {
      const max = Math.max(old.length, prompt.length)
      this.term.write('\r' + ' '.repeat(max) + '\r' + prompt)
    }
  }

  sendCommand(text: string) {
    if (this.busy) {
      this.cmdQueue.push(text)
      return
    }
    if (text === 'clear') {
      this.term.write('\r\n')
      this.term.clear()
      this.writePrompt()
      return
    }
    if (text === 'repl') {
      this.busy = true
      this.term.write('\r\n')
      this.sendRaw(JSON.stringify({ input: text }))
      return
    }
    this.busy = true
    this.interactive = text.startsWith('os ')
    this.term.write('\r\n')
    this.sendRaw(JSON.stringify({ resize: true, cols: this.term.cols, rows: this.term.rows }))
    this.sendRaw(JSON.stringify({ input: text }))
  }

  private sequenceHandlers: Record<string, () => void> = {
    '\r': () => this.handleEnter(),
    '\x7f': () => this.handleBackspace(),
    '\b': () => this.handleBackspace(),
    '\x1b[A': () => this.handleArrowUp(),
    '\x1bOA': () => this.handleArrowUp(),
    '\x1b[B': () => this.handleArrowDown(),
    '\x1bOB': () => this.handleArrowDown(),
    '\x1b[D': () => this.handleArrowLeft(),
    '\x1bOD': () => this.handleArrowLeft(),
    '\x1b[C': () => this.handleArrowRight(),
    '\x1bOC': () => this.handleArrowRight(),
  }

  handleTerminalData(data: string) {
    if (this.busy && this.interactive) {
      this.sendRaw(JSON.stringify({ input: data }))
      return
    }
    if (this.busy) return

    const handler = this.sequenceHandlers[data]
    if (handler) {
      handler()
      return
    }

    if (data >= ' ') {
      this.insertChar(data)
    }
  }

  private handleEnter() {
    const line = this.currentLine
    this.currentLine = ''
    this.cursorPos = 0
    this.historyIndex = -1
    if (line.trim()) {
      this.history.push(line)
      this.sendCommand(line)
    } else {
      this.term.write('\r\n')
      this.writePrompt()
    }
  }

  private handleBackspace() {
    if (this.cursorPos <= 0) return
    this.cursorPos--
    const before = this.currentLine.slice(0, this.cursorPos)
    const after = this.currentLine.slice(this.cursorPos + 1)
    this.currentLine = before + after
    const tail = this.currentLine.slice(this.cursorPos)
    this.term.write('\b' + tail + ' ' + '\b'.repeat(tail.length + 1))
  }

  private handleArrowUp() {
    if (this.history.length === 0) return
    if (this.historyIndex === -1) {
      this.historyIndex = this.history.length - 1
    } else if (this.historyIndex > 0) {
      this.historyIndex--
    } else {
      return
    }
    this.renderHistoryEntry()
  }

  private handleArrowDown() {
    if (this.historyIndex === -1) return
    const max = this.currentLine.length + this.prompt.length
    if (this.historyIndex < this.history.length - 1) {
      this.historyIndex++
      this.renderHistoryEntry()
    } else {
      this.historyIndex = -1
      this.term.write('\r' + ' '.repeat(max) + '\r')
      this.writePrompt()
      this.currentLine = ''
      this.cursorPos = 0
    }
  }

  private renderHistoryEntry() {
    const entry = this.history[this.historyIndex]
    const max = this.currentLine.length + this.prompt.length
    this.term.write('\r' + ' '.repeat(max) + '\r')
    this.writePrompt()
    this.term.write(entry)
    this.currentLine = entry
    this.cursorPos = entry.length
  }

  private handleArrowLeft() {
    if (this.cursorPos > 0) {
      this.cursorPos--
      this.term.write('\b')
    }
  }

  private handleArrowRight() {
    if (this.cursorPos < this.currentLine.length) {
      this.cursorPos++
      this.term.write('\x1b[C')
    }
  }

  private insertChar(data: string) {
    if (this.cursorPos === this.currentLine.length) {
      this.currentLine += data
      this.term.write(data)
    } else {
      const before = this.currentLine.slice(0, this.cursorPos)
      const after = this.currentLine.slice(this.cursorPos)
      this.currentLine = before + data + after
      this.term.write(data + after + '\b'.repeat(after.length))
    }
    this.cursorPos++
  }

  handleFinalize() {
    this.busy = false
    this.interactive = false

    if (this.cmdQueue.length > 0) {
      this.term.write('\r\n')
      const text = this.cmdQueue.shift()!
      this.busy = true
      this.sendRaw(JSON.stringify({ input: text }))
      return
    }

    this.writePrompt()
  }

  handleSpawned(pid: number, dispatch: (action: any) => void) {
    const pending = this.pendingRepl
    if (!pending) return
    this.pendingRepl = null
    dispatch({
      type: 'OPEN_INTERACTIVE_TAB',
      tab: {
        pid,
        binary: 'repl',
        sessionId: pending.sessionId,
        mode: 'pty',
        createdAt: Date.now(),
      },
    })
    this.busy = false
    this.interactive = false
    this.writePrompt()
  }
}
