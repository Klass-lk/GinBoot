Feature: TestSuite steps

  Background:
    Given document "items" has the following items
      | id | name  | price | active |
      | 1  | Chalk | 2.5   | true   |

  Scenario: Table seeding, JSON seeding and path assertions
    Given document "notes" has the following JSON:
      """
      [{"_id": "n1", "text": "hello", "tags": ["a", "b"]}]
      """
    When I send a GET request to "/items/1"
    Then the response status should be 200
    And the response "name" should be "Chalk"
    And the response "$.price" should be "2.5"
    And the response "$.tags" should be "[\"x\",\"y\"]"
    And the response "$.tags" should have 2 items
    And the response "$.missing" should not exist
    And the response "$.nothing" should be null
    And the response "$.name" should exist
    And the response header "Content-Type" should be "application/json; charset=utf-8"
    And the response "name" field is stored as "itemName"
    When I send a GET request to "/echo/{{itemName}}"
    Then the response "$.value" should be "Chalk"
    When I send a GET request to "/notes"
    Then the response "$.count" should be "1"

  Scenario: Principals, headers and cookies
    Given I am authenticated as "admin"
    And I set the request header "X-Trace" to "t-1"
    And I set the cookie "device" to "d-1"
    When I send a GET request to "/whoami"
    Then the response "$.auth" should be "Bearer admin-token"
    And the response "$.trace" should be "t-1"
    And the response "$.device" should be "d-1"
    Given I am authenticated as "minted"
    When I send a GET request to "/whoami"
    Then the response "$.auth" should be "Bearer minted-for-minted"
    Given I am not authenticated
    When I send a GET request to "/whoami"
    Then the response "$.auth" should be ""

  Scenario: JSON bodies
    When I send a POST request to "/echo" with JSON:
      """
      {"value": "posted"}
      """
    Then the response "$.value" should be "posted"

  Scenario: Comparing with a reference service
    When I send a GET request to "/items/1" to both services
    Then both responses should match ignoring:
      | $.generatedAt |
    When I send a GET request to "/drift" to both services
    Then both responses should match ignoring:
      | $.version |

  Scenario: Snapshots
    When I send a GET request to "/items/1"
    Then the response should match snapshot "item-1"
