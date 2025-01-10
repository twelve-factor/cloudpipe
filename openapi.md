```yaml
openapi: 3.0.0
info:
  title: CloudPipe API
  description: API to allow a broker to configure connection information for a given resource, including protocol, authentication, end encryption.
  version: 0.1.0

paths:
  /pipes:
    post:
      summary: Create a new pipe
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Pipe'
            example:
              id: frontend
              this:
                data:
                  URI: https://backend.herokuapp.com
              other:
                uri: https://api.heroku.com/apps/frontend/pipes/backend
                issuer: https://oidc.heroku.com

      responses:
        '201':
          description: Pipe created successfully
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Pipe'
              example:
                id: frontend
                this:
                  uri: https://api.heroku.com/apps/backend/pipes/frontend
                  issuer: https://oidc.heroku.com
                  data:
                    URI: https://backend.herokuapp.com
                other:
                  uri: https://frontend.herokuapp.com/pipes/backend
                  issuer: https://oidc.heroku.com
                _links:
                  self:
                    href: https://api.heroku.com/apps/backend/pipes/frontend
    get:
      summary: Retrieve all pipes
      responses:
        '200':
          description: A list of pipes
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: '#/components/schemas/Pipe'
              example:
                - id: frontend
                  this:
                    uri: https://api.heroku.com/apps/backend/pipes/frontend
                    issuer: https://oidc.heroku.com
                    data:
                      URI: https://backend.herokuapp.com
                  other:
                    uri: https://frontend.herokuapp.com/pipes/backend
                    issuer: https://oidc.heroku.com
                  _links:
                    self:
                      href: https://api.heroku.com/apps/backend/pipes/frontend
  /pipes/{pipeid}:
    get:
      summary: Retrieve a specific pipe by ID
      parameters:
        - name: pipeid
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: Pipe details
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Pipe'
              example:
                id: frontend
                this:
                  uri: https://api.heroku.com/apps/backend/pipes/frontend
                  issuer: https://oidc.heroku.com
                  data:
                    URI: https://backend.herokuapp.com
                other:
                  uri: https://frontend.herokuapp.com/pipes/backend
                  issuer: https://oidc.heroku.com
                _links:
                  self:
                    href: https://api.heroku.com/apps/backend/pipes/frontend
    patch:
      summary: Update a specific pipe by ID
      parameters:
        - name: pipeid
          in: path
          required: true
          schema:
            type: string
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Pipe'
            example:
              this:
                data:
                  URI: https://updated.herokuapp.com
      responses:
          '202':
            description: Pipe updated successfully
    delete:
      summary: Delete a specific pipe by ID
      parameters:
        - name: pipeid
          in: path
          required: true
          schema:
            type: string
      responses:
        '204':
          description: Pipe deleted successfully

  /offers:
    get:
      summary: Retrieve all offers
      responses:
        '200':
          description: A list of offers
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: '#/components/schemas/Blueprint'


  /offers/{blueprintid}:
    get:
      summary: Retrieve a specific offer blueprint by blueprint ID
      parameters:
        - name: blueprintid
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: Offer blueprint details
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Blueprint'

  /offers/{blueprintid}/bindings:
    post:
      summary: Create a new pipe from blueprint
      parameters:
        - name: blueprintid
          in: path
          required: true
          schema:
            type: string
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Binding'
      responses:
        '202':
          description: Pipe created successfully
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Binding'

  /offers/{blueprintid}/protos/{templateid}:
    get:
      summary: Retrieve a specific proto template by blueprint and template ID
      parameters:
        - name: blueprintid
          in: path
          required: true
          schema:
            type: string
        - name: templateid
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: Proto template details
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/PipeTemplate'

  /offers/{blueprintid}/adapters/{templateid}:
    get:
      summary: Retrieve a specific adapter template by blueprint and template ID
      parameters:
        - name: blueprintid
          in: path
          required: true
          schema:
            type: string
        - name: templateid
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: Adapter template details
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/PipeTemplate'

  /needs:
    get:
      summary: Retrieve all needs
      responses:
        '200':
          description: A list of needs
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: '#/components/schemas/Blueprint'

  /needs/{blueprintid}:
    get:
      summary: Retrieve a specific need blueprint by blueprint ID
      parameters:
        - name: blueprintid
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: Need blueprint details
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Blueprint'

  /needs/{blueprintid}/bindings:
    post:
      summary: Create a new pipe from blueprint
      parameters:
        - name: blueprintid
          in: path
          required: true
          schema:
            type: string
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Binding'
      responses:
        '202':
          description: Pipe created successfully
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Binding'

  /needs/{blueprintid}/protos/{templateid}:
    get:
      summary: Retrieve a specific proto template by blueprint and template ID
      parameters:
        - name: blueprintid
          in: path
          required: true
          schema:
            type: string
        - name: templateid
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: Proto template details
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/PipeTemplate'

  /needs/{blueprintid}/adapters/{templateid}:
    get:
      summary: Retrieve a specific adapter template by blueprint and template ID
      parameters:
        - name: blueprintid
          in: path
          required: true
          schema:
            type: string
        - name: templateid
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: Adapter template details
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/PipeTemplate'

components:
  schemas:
    PipeTemplate:
      type: object
      properties:
        id:
          type: string
          description: id of the template
        this:
          type: object
          description: jsonschema for this end of the pipe
        other:
          type: object
          description: jsonschema for this end of the pipe

    Blueprint:
      type: object
      properties:
        name:
          type: string
        adapters:
          type: array
          description: prioritized list of supported adapters
          items:
            $ref: '#/components/schemas/PipeTemplate'
        defaultAdapters:
          type: array
          description: adapters to include by default if not specified
          items:
            type: string
        protos:
          type: array
          description: prioritized list of supported protocols
          items:
            $ref: '#/components/schemas/PipeTemplate'
        maxPipes:
          type: integer

    End:
      type: object
      description: one end of a pipe
      properties:
        issuer:
          type: string
          description: oidc issuer for the owning broker of this pipe
        uri:
          type: string
          description: uri of this end of the pipe
        schema:
          type: object
          description: jsonschema that the data must follow
        data:
          type: object
          description: arbitrary json data

    Link:
      type: object
      description: standard HAL link object
      properties:
        href:
          type: string
        title:
          type: string
        templated:
          type: boolean

    Links:
      type: object
      description: _links used for the Pipe object
      properties:
        self:
          $ref: '#/components/schemas/Link'
        blueprint:
          $ref: '#/components/schemas/Link'
        adapters:
          items:
            $ref: '#/components/schemas/Link'
        proto:
          $ref: '#/components/schemas/Link'

    Pipe:
      type: object
      properties:
        id:
          type: string
        this:
          $ref: '#/components/schemas/End'
        other:
          $ref: '#/components/schemas/End'
        _links:
          $ref: '#/components/schemas/Links'

    Binding:
      type: object
      properties:
        pipe:
          $ref: '#/components/schemas/Pipe'
        adapters:
          description: adapters to use for the pipe
          items:
            type: string
        proto:
          type: string
          description: protocol to use for the pipe
```
