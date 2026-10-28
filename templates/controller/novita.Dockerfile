FROM novitalabs/github-actions-controller:v0.1.1
USER root
COPY templates/controller/entrypoint.sh /usr/local/bin/novita-actions-controller-entrypoint
RUN chmod 0755 /usr/local/bin/novita-actions-controller-entrypoint
USER user
ENTRYPOINT ["/usr/local/bin/novita-actions-controller-entrypoint"]
CMD []
