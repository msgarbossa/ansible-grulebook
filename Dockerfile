FROM scratch
EXPOSE 5000
COPY ./out/ansible-grulebook-*-linux-amd64 /ansible-grulebook
ENTRYPOINT [ "/ansible-grulebook" ]
