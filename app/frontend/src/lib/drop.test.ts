import { describe, expect, it } from 'vitest'
import { dropText, quotePath } from './drop'

describe('quotePath', () => {
  it('leaves plain paths alone', () => {
    expect(quotePath('/Users/me/Desktop/shot-1.png')).toBe('/Users/me/Desktop/shot-1.png')
  })

  it('escapes spaces and shell characters like Terminal.app', () => {
    expect(quotePath('/tmp/Captura de pantalla (2).png')).toBe('/tmp/Captura\\ de\\ pantalla\\ \\(2\\).png')
    expect(quotePath("/tmp/it's $HOME&x;y")).toBe("/tmp/it\\'s\\ \\$HOME\\&x\\;y")
  })

  it('keeps non-ASCII letters', () => {
    expect(quotePath('/tmp/año/ñandú.jpg')).toBe('/tmp/año/ñandú.jpg')
  })
})

describe('dropText', () => {
  it('joins the paths with a trailing space', () => {
    expect(dropText(['/a.png', '/b c.png'])).toBe('/a.png /b\\ c.png ')
  })

  it('is empty when no file was dropped', () => {
    expect(dropText([])).toBe('')
    expect(dropText([''])).toBe('')
  })
})
