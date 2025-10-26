Feature: Posts
  Manage posts in the application

  Background: Setup
    Given document "posts" has the following items
      | id | title | content |
      | 1  | Post 1| Content 1 |

  Scenario: Create a new post
    When I send a POST request to "/posts" with body
      | title   | content   |
      | New Post| New Content |
    Then the response status should be 200
    And the response should contain an item with
      | title   | content   |
      | New Post| New Content |

  Scenario: Get a post
    When I send a GET request to "/posts/1"
    Then the response status should be 200
    And the response should contain an item with
      | id    | title  | content   |
      | 1     | Post 1 | Content 1 |

  Scenario: Get all posts
    When I send a GET request to "/posts"
    Then the response status should be 200

  Scenario: Update a post
    When I send a PUT request to "/posts/1" with body
      | title        | content        |
      | Updated Post | Updated Content |
    Then the response status should be 200
    And the response should contain an item with
      | title        | content        |
      | Updated Post | Updated Content |

  Scenario: Delete a post
    When I send a DELETE request to "/posts/1"
    Then the response status should be 200
