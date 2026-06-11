movdqu (%rax,%rdi), %xmm0
movdqu (%rbx,%rdi), %xmm1
pcmpeqb %xmm1, %xmm0
pmovmskb %xmm0, %r9d
cmpl $0xffff, %r9d
addq $16, %rdi
