.386p

_DATA segment byte public use16 'DATA'

msg db 'Hello world!$'

_DATA ends

_TEXT segment byte public use16 'CODE'
      assume cs:_TEXT

public _greeting
_greeting proc near
          push bp
          mov bp,sp

          ; mov ax,seg _DATA
          ; mov ds,ax
          mov ah,09h
          mov dx,offset msg
          int 21h

          pop bp
          mov ax,0x4c08
          int 21h
_greeting endp

_TEXT ends
      end

