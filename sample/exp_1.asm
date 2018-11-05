.model small
.386p

.stack 100h 

.code
start:
  ; do 'mov ax,4c01h'
  mov ax,4ch
  shl ax,8
  add ax,01h
  ;
  int 21h 

end start  
