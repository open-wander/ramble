FROM debian:buster-slim
RUN apt update && apt -y install curl
# Create Conf Directory
RUN mkdir /etc/rmbl
# configuration volume
VOLUME /etc/rmbl
# Set the releae environment for gin
ENV GIN_MODE="release"
COPY rmbl-server /
# EXPOSE 10000
ENTRYPOINT ["/rmbl-server"]
