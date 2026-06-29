import { useState, useEffect, useRef, useCallback } from 'react'
import { useStore } from '../state'
import { sendAgentInput } from '../api/pool'

export function AgentModal() {
  const { state, dispatch } = useStore()
  const [output, setOutput] = useState('')
  const [input, setInput] = useState('')
  const esRef = useRef<EventSource | null>(null)
  const outputRef = useRef<HTMLDivElement>(null)

  const close = useCallback(() => {
    if (esRef.current) {
      esRef.current.close()
      esRef.current = null
    }
    dispatch({ type: 'CLOSE_MODAL' })
    setOutput('')
  }, [dispatch])

  useEffect(() => {
    if (!state.modalOpen || !state.modalPid || !state.accessToken) return

    const url = '/api/agent/' + state.modalPid + '/stream?token=' + encodeURIComponent(state.accessToken)
    const es = new EventSource(url)
    esRef.current = es

    es.onmessage = (e) => {
      let data: { type?: string; text?: string }
      try {
        data = JSON.parse(e.data)
      } catch {
        return
      }
      if (data.type === 'exit') {
        es.close()
        esRef.current = null
        return
      }
      setOutput(prev => prev + (data.text || '').replace(/\x1b\[[0-9;]*[a-zA-Z]/g, '') + '\n')
    }

    es.onerror = () => {
      es.close()
      esRef.current = null
    }

    return () => {
      es.close()
    }
  }, [state.modalOpen, state.modalPid, state.accessToken])

  useEffect(() => {
    if (outputRef.current) {
      outputRef.current.scrollTop = outputRef.current.scrollHeight
    }
  }, [output])

  const handleKeyDown = async (e: React.KeyboardEvent) => {
    if (e.key !== 'Enter' || !state.modalPid || !state.accessToken) return
    const text = input
    setInput('')
    setOutput(prev => prev + '> ' + text + '\n')
    try {
      await sendAgentInput(state.modalPid, text, state.accessToken)
    } catch (err) {
      setOutput(prev => prev + 'error: ' + (err instanceof Error ? err.message : String(err)) + '\n')
    }
  }

  if (!state.modalOpen) return null

  return (
    <div id="agent-modal" className="modal">
      <div className="modal-backdrop" onClick={close} />
      <div className="modal-content">
        <div className="modal-header">
          <span className="modal-title" id="modal-title">
            PID {state.modalPid} — {state.modalBinary} ({state.modalSessionId})
          </span>
          <span className="modal-close" id="modal-close" onClick={close}>
            &times;
          </span>
        </div>
        <div className="modal-output" ref={outputRef}>
          {output}
        </div>
        <div className="modal-input-wrap">
          <span className="modal-prompt">&gt;</span>
          <input
            className="modal-input"
            type="text"
            placeholder="Type input for the agent..."
            autoFocus
            value={input}
            onChange={e => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
          />
        </div>
      </div>
    </div>
  )
}
