.386p

_DATA segment byte public use16 'DATA'

msg db 'Hello world!$'

_DATA ends

_TEXT segment byte public use16 'CODE'
      assume cs:_TEXT

public _hello
_hello    proc near
          push bp
          mov bp,sp
          ; push ds

          mov ah,09h
          mov dx,offset msg
          int 21h

          ; pop ds
          mov sp,bp
          pop bp
          mov ax,0x4c00
          int 21h
_hello    endp

_TEXT ends
      end

