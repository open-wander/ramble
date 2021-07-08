FROM debian:buster-slim
RUN apt update && apt -y install curl
# Create Conf Directory
RUN mkdir /etc/rmbl
# configuration volume
VOLUME /etc/rmbl
COPY rmbl-server /
RUN chmod +x /rmbl-server
# EXPOSE 10000
ENTRYPOINT ["/rmbl-server"]
